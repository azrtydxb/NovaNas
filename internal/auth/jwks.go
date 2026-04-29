package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

// jwk is a single JSON Web Key. We only parse what we need to verify
// signatures: RSA (RS256/384/512, PS*) and EC (ES256/384/512). Other
// kty values are skipped silently.
//
// See RFC 7517 (JWK) and RFC 7518 (JWA) for field semantics.
type jwk struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Alg string `json:"alg,omitempty"`
	Use string `json:"use,omitempty"`

	// RSA
	N string `json:"n,omitempty"`
	E string `json:"e,omitempty"`

	// EC
	Crv string `json:"crv,omitempty"`
	X   string `json:"x,omitempty"`
	Y   string `json:"y,omitempty"`
}

type jwksDoc struct {
	Keys []jwk `json:"keys"`
}

type oidcDiscovery struct {
	JWKSURI string `json:"jwks_uri"`
	Issuer  string `json:"issuer"`
}

// jwksCache fetches and caches the JWKS document for an issuer. It
// performs OIDC discovery (.well-known/openid-configuration) once to
// resolve jwks_uri, then refreshes the JWKS on TTL expiry or on a
// missing-kid lookup (single forced refresh, to handle key rotation
// gracefully).
type jwksCache struct {
	issuer string
	ttl    time.Duration
	httpc  *http.Client

	mu        sync.Mutex
	jwksURI   string
	keys      map[string]any // kid -> *rsa.PublicKey | *ecdsa.PublicKey
	loadedAt  time.Time
	lastForce time.Time // throttle for forced refreshes on miss
}

func newJWKSCache(issuer string, ttl time.Duration, httpc *http.Client) *jwksCache {
	if httpc == nil {
		httpc = http.DefaultClient
	}
	return &jwksCache{issuer: issuer, ttl: ttl, httpc: httpc}
}

// keyForKID returns the public key for kid, refreshing the JWKS if the
// kid is unknown or the cache has expired.
func (c *jwksCache) keyForKID(ctx context.Context, kid string) (any, error) {
	c.mu.Lock()
	expired := c.keys == nil || time.Since(c.loadedAt) > c.ttl
	if !expired {
		if k, ok := c.keys[kid]; ok {
			c.mu.Unlock()
			return k, nil
		}
	}
	c.mu.Unlock()

	if err := c.refresh(ctx); err != nil {
		return nil, err
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	if k, ok := c.keys[kid]; ok {
		return k, nil
	}
	return nil, fmt.Errorf("auth: no JWK with kid %q", kid)
}

func (c *jwksCache) refresh(ctx context.Context) error {
	c.mu.Lock()
	jwksURI := c.jwksURI
	c.lastForce = time.Now()
	c.mu.Unlock()

	if jwksURI == "" {
		uri, err := c.discoverJWKSURI(ctx)
		if err != nil {
			return err
		}
		jwksURI = uri
	}

	keys, err := c.fetchJWKS(ctx, jwksURI)
	if err != nil {
		return err
	}

	c.mu.Lock()
	c.jwksURI = jwksURI
	c.keys = keys
	c.loadedAt = time.Now()
	c.mu.Unlock()
	return nil
}

func (c *jwksCache) discoverJWKSURI(ctx context.Context) (string, error) {
	url := strings.TrimRight(c.issuer, "/") + "/.well-known/openid-configuration"
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return "", err
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return "", fmt.Errorf("auth: oidc discovery: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("auth: oidc discovery: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return "", err
	}
	var d oidcDiscovery
	if err := json.Unmarshal(body, &d); err != nil {
		return "", fmt.Errorf("auth: oidc discovery: %w", err)
	}
	if d.JWKSURI == "" {
		return "", errors.New("auth: oidc discovery: missing jwks_uri")
	}
	return d.JWKSURI, nil
}

func (c *jwksCache) fetchJWKS(ctx context.Context, jwksURI string) (map[string]any, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURI, nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.httpc.Do(req)
	if err != nil {
		return nil, fmt.Errorf("auth: jwks fetch: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("auth: jwks fetch: status %d", resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return nil, err
	}
	var doc jwksDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("auth: jwks parse: %w", err)
	}
	out := make(map[string]any, len(doc.Keys))
	for _, k := range doc.Keys {
		pub, err := parseJWK(k)
		if err != nil {
			// Skip unknown/unparseable keys silently — IdPs sometimes
			// publish keys for algs we don't support.
			continue
		}
		if k.Kid == "" {
			// Without a kid we can't dispatch. Skip.
			continue
		}
		out[k.Kid] = pub
	}
	if len(out) == 0 {
		return nil, errors.New("auth: jwks: no usable keys")
	}
	return out, nil
}

func parseJWK(k jwk) (any, error) {
	switch k.Kty {
	case "RSA":
		if k.N == "" || k.E == "" {
			return nil, errors.New("rsa jwk missing n/e")
		}
		nB, err := base64.RawURLEncoding.DecodeString(k.N)
		if err != nil {
			return nil, err
		}
		eB, err := base64.RawURLEncoding.DecodeString(k.E)
		if err != nil {
			return nil, err
		}
		e := 0
		for _, b := range eB {
			e = e<<8 | int(b)
		}
		if e == 0 {
			return nil, errors.New("rsa jwk: zero exponent")
		}
		return &rsa.PublicKey{N: new(big.Int).SetBytes(nB), E: e}, nil
	case "EC":
		var curve elliptic.Curve
		switch k.Crv {
		case "P-256":
			curve = elliptic.P256()
		case "P-384":
			curve = elliptic.P384()
		case "P-521":
			curve = elliptic.P521()
		default:
			return nil, fmt.Errorf("ec jwk: unsupported crv %q", k.Crv)
		}
		xB, err := base64.RawURLEncoding.DecodeString(k.X)
		if err != nil {
			return nil, err
		}
		yB, err := base64.RawURLEncoding.DecodeString(k.Y)
		if err != nil {
			return nil, err
		}
		return &ecdsa.PublicKey{
			Curve: curve,
			X:     new(big.Int).SetBytes(xB),
			Y:     new(big.Int).SetBytes(yB),
		}, nil
	default:
		return nil, fmt.Errorf("unsupported kty %q", k.Kty)
	}
}
