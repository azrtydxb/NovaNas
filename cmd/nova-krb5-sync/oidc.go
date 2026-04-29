// OIDC client_credentials helper for nova-krb5-sync. Mirrors the
// implementation in cmd/nova-csi/oidc.go (kept as a sibling package
// rather than shared because both daemons are tiny and the indirection
// cost of a shared package outweighs the benefit).
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
	"time"
)

type novaTokenSource struct {
	tokenURL     string
	clientID     string
	clientSecret string
	httpc        *http.Client
}

func (s *novaTokenSource) init(caPEM []byte) error {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
	if len(caPEM) > 0 {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return errors.New("oidc: CA PEM contained no valid certificates")
		}
		tlsCfg.RootCAs = pool
	}
	s.httpc = &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: tlsCfg},
	}
	return nil
}

func (s *novaTokenSource) fetch(ctx context.Context) (string, time.Time, error) {
	form := url.Values{}
	form.Set("grant_type", "client_credentials")
	form.Set("client_id", s.clientID)
	form.Set("client_secret", s.clientSecret)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.tokenURL, strings.NewReader(form.Encode()))
	if err != nil {
		return "", time.Time{}, err
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
		return "", time.Time{}, fmt.Errorf("oidc: token endpoint returned %d: %s", resp.StatusCode, truncate(string(body), 256))
	}
	var tr struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tr); err != nil {
		return "", time.Time{}, fmt.Errorf("oidc: decode token: %w", err)
	}
	if tr.AccessToken == "" {
		return "", time.Time{}, errors.New("oidc: empty access_token")
	}
	exp := time.Time{}
	if jwtExp, ok := decodeJWTExp(tr.AccessToken); ok {
		exp = jwtExp
	} else if tr.ExpiresIn > 0 {
		exp = time.Now().Add(time.Duration(tr.ExpiresIn) * time.Second)
	} else {
		exp = time.Now().Add(60 * time.Second)
	}
	return tr.AccessToken, exp, nil
}

type tokenSink interface {
	SetToken(string)
}

func (s *novaTokenSource) runRefresh(ctx context.Context, sink tokenSink, initialExp time.Time, logger *slog.Logger) {
	exp := initialExp
	for {
		remaining := time.Until(exp)
		if remaining <= 0 {
			remaining = 10 * time.Second
		}
		wait := time.Duration(float64(remaining) * 0.7)
		if wait < 10*time.Second {
			wait = 10 * time.Second
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		attempt := 0
		for {
			fetchCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			tok, newExp, err := s.fetch(fetchCtx)
			cancel()
			if err == nil {
				sink.SetToken(tok)
				exp = newExp
				logger.Info("nova-api oidc token refreshed", "exp", exp.UTC().Format(time.RFC3339))
				break
			}
			attempt++
			backoff := time.Duration(math.Min(float64(30*time.Second), float64(2*time.Second)*math.Pow(2, float64(attempt-1))))
			backoff += time.Duration(rand.Int64N(int64(time.Second)))
			if rem := time.Until(exp); rem > time.Second && backoff > rem {
				backoff = rem
			}
			logger.Warn("nova-api oidc token refresh failed; retrying", "attempt", attempt, "backoff", backoff.String(), "err", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
		}
	}
}

func decodeJWTExp(token string) (time.Time, bool) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return time.Time{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
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
