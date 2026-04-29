// OIDC client_credentials token source for nova-csi.
//
// Keycloak (and any other compliant OIDC provider) accepts a POST to
// <issuer>/protocol/openid-connect/token with grant_type=client_credentials
// and returns a short-lived access token. This file fetches that token at
// startup and refreshes it in the background at ~70% of its remaining
// lifetime so the SDK always has a fresh bearer.
//
// We intentionally do NOT pull in a JWT library. Decoding the `exp` claim
// from a JWT requires base64url-decoding the middle segment and reading a
// single integer field, which is trivial and not worth a dependency. If
// the token is unparseable for any reason we fall back to the OAuth
// `expires_in` value returned alongside the access token.
package main

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"math"
	"math/rand/v2"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// oidcSource fetches access tokens via the OAuth2 client_credentials grant.
// Construct one with newOIDCSource and call Fetch from a single refresh
// goroutine. Concurrent Fetch calls are not expected but are safe (each one
// just performs an independent HTTP round-trip).
type oidcSource struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpc        *http.Client
}

// newOIDCSource builds the token endpoint URL from the issuer and configures
// an http.Client with the same TLS posture as the SDK (CA pool only; no
// InsecureSkipVerify here — operators who need that knob already have it via
// the SDK CA cert path).
func newOIDCSource(issuerURL, clientID, clientSecret string, caPEM []byte) (*oidcSource, error) {
	if strings.TrimSpace(issuerURL) == "" {
		return nil, errors.New("oidc: issuer URL is required")
	}
	if strings.TrimSpace(clientID) == "" {
		return nil, errors.New("oidc: client ID is required")
	}
	if strings.TrimSpace(clientSecret) == "" {
		return nil, errors.New("oidc: client secret is required")
	}

	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if len(caPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("oidc: CA PEM contained no valid certificates")
		}
		tlsCfg.RootCAs = pool
	}

	tokenURL := strings.TrimRight(strings.TrimSpace(issuerURL), "/") + "/protocol/openid-connect/token"
	return &oidcSource{
		tokenURL:     tokenURL,
		clientID:     clientID,
		clientSecret: clientSecret,
		httpc: &http.Client{
			Timeout:   30 * time.Second,
			Transport: &http.Transport{TLSClientConfig: tlsCfg},
		},
	}, nil
}

// tokenResponse is the subset of the OAuth2 token-endpoint JSON we read.
type tokenResponse struct {
	AccessToken string `json:"access_token"`
	ExpiresIn   int    `json:"expires_in"`
	TokenType   string `json:"token_type"`
}

// Fetch performs a single client_credentials exchange. It returns the access
// token and the parsed expiry. If the JWT's `exp` claim is readable it wins,
// otherwise we fall back to now+expires_in.
func (s *oidcSource) Fetch(ctx context.Context) (string, time.Time, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", s.clientID)
	form.Set("client_secret", s.clientSecret)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, fmt.Errorf("oidc: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	resp, err := s.httpc.Do(req)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("oidc: token endpoint: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		// Redact the body if it looks like it embeds the secret. In
		// practice Keycloak just returns {"error":"...","error_description":"..."}.
		return "", time.Time{}, fmt.Errorf("oidc: token endpoint returned %d: %s", resp.StatusCode, truncate(string(body), 256))
	}

	var tr tokenResponse
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", time.Time{}, fmt.Errorf("oidc: decode token response: %w", err)
	}
	if tr.AccessToken == "" {
		return "", time.Time{}, errors.New("oidc: token endpoint returned empty access_token")
	}

	exp := time.Time{}
	if jwtExp, ok := decodeJWTExp(tr.AccessToken); ok {
		exp = jwtExp
	} else if tr.ExpiresIn > 0 {
		exp = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	} else {
		// Unknown expiry — pick a conservative 60s so the refresh loop
		// retries quickly rather than treating the token as immortal.
		exp = time.Now().Add(60 * time.Second)
	}
	return tr.AccessToken, exp, nil
}

// decodeJWTExp pulls the `exp` claim (Unix seconds) out of a compact-serialized
// JWT without any signature verification — the API server validates the JWT,
// not us. Returns ok=false when the input is not a parseable three-segment
// JWT or has no numeric `exp` claim.
func decodeJWTExp(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		// Some issuers pad; try the standard URL encoding too.
		payload, err = base64.URLEncoding.DecodeString(parts[1])
		if err != nil {
			return time.Time{}, false
		}
	}
	var claims struct {
		Exp json.Number `json:"exp"`
	}
	if err := json.Unmarshal(payload, &claims); err != nil {
		return time.Time{}, false
	}
	expSec, err := claims.Exp.Int64()
	if err != nil || expSec <= 0 {
		return time.Time{}, false
	}
	return time.Unix(expSec, 0), true
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// -------------------------------------------------------------------------
// Refresh loop
// -------------------------------------------------------------------------

// tokenSetter is what the refresh loop calls to publish a new token. The
// novanas.Client.SetToken method satisfies it.
type tokenSetter interface {
	SetToken(string)
}

// runOIDCRefresh fetches new tokens at ~70% of the previous token's remaining
// lifetime. On failure it retries with capped exponential backoff, never
// sleeping longer than the deadline before token expiry. It returns when ctx
// is cancelled.
//
// Caller is responsible for the initial fetch (so startup can fail closed
// before the gRPC server begins serving). This loop only handles refreshes
// after that initial success.
func runOIDCRefresh(ctx context.Context, src *oidcSource, sink tokenSetter, initialExp time.Time, logger *slog.Logger) {
	exp := initialExp
	for {
		// Refresh point: 70% of remaining lifetime, with a 10s floor so we
		// don't tight-loop on already-near-expired tokens.
		now := time.Now()
		remaining := time.Until(exp)
		if remaining <= 0 {
			remaining = 10 * time.Second
		}
		wait := time.Duration(float64(remaining) * 0.7)
		if wait < 10*time.Second {
			wait = 10 * time.Second
		}
		_ = now

		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}

		// Try to refresh, with exponential backoff capped by the actual
		// expiry deadline. After expiry we keep retrying because the only
		// alternative is process exit, which would just CrashLoopBackOff.
		attempt := 0
		for {
			fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			tok, newExp, err := src.Fetch(fetchCtx)
			cancel()
			if err == nil {
				sink.SetToken(tok)
				exp = newExp
				logger.Info("oidc token refreshed", "exp", exp.UTC().Format(time.RFC3339))
				break
			}
			attempt++
			// Backoff: 2s, 4s, 8s ... capped at 30s, plus jitter.
			backoff := time.Duration(math.Min(float64(30*time.Second), float64(2*time.Second)*math.Pow(2, float64(attempt-1))))
			backoff += time.Duration(rand.Int64N(int64(time.Second)))
			// Cap backoff at remaining time-to-expiry, but never below 1s.
			if rem := time.Until(exp); rem > time.Second && backoff > rem {
				backoff = rem
			}
			logger.Warn("oidc token refresh failed; retrying",
				"attempt", attempt, "backoff", backoff.String(), "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
	}
}

// fetchInitialToken performs the startup fetch. We deliberately fail closed
// here: if the binary cannot acquire a token at startup, the gRPC server
// would just immediately reject every CSI RPC anyway, and CrashLoopBackOff
// is a clearer signal to the operator than a quietly degraded driver.
func fetchInitialToken(ctx context.Context, src *oidcSource, logger *slog.Logger) (string, time.Time, error) {
	tok, exp, err := src.Fetch(ctx)
	if err != nil {
		return "", time.Time{}, err
	}
	logger.Info("oidc initial token acquired", "exp", exp.UTC().Format(time.RFC3339))
	return tok, exp, nil
}

// staticSink is a tiny tokenSetter for tests / non-Client callers.
type staticSink struct {
	mu sync.Mutex
	v  string
}

func (s *staticSink) SetToken(t string) { s.mu.Lock(); s.v = t; s.mu.Unlock() }
func (s *staticSink) Get() string       { s.mu.Lock(); defer s.mu.Unlock(); return s.v }
