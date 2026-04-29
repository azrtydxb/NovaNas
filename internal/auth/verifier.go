package auth

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Identity is what a verified JWT projects to.
type Identity struct {
	Subject       string    `json:"sub"`
	PreferredName string    `json:"preferredUsername"`
	Email         string    `json:"email,omitempty"`
	Realm         string    `json:"realm"`
	Roles         []string  `json:"roles"`
	Scopes        []string  `json:"scopes"`
	ExpiresAt     time.Time `json:"expiresAt"`
}

// Verifier validates tokens.
type Verifier struct {
	cfg    Config
	keyset *jwksCache
	httpc  *http.Client
}

// NewVerifier constructs a Verifier. httpc may be nil to use http.DefaultClient.
func NewVerifier(cfg Config, httpc *http.Client) (*Verifier, error) {
	cfg.withDefaults()
	if !cfg.SkipVerify {
		if cfg.IssuerURL == "" {
			return nil, errors.New("auth: IssuerURL is required")
		}
		if cfg.Audience == "" {
			return nil, errors.New("auth: Audience is required")
		}
	}
	if httpc == nil {
		httpc = http.DefaultClient
	}
	v := &Verifier{cfg: cfg, httpc: httpc}
	if !cfg.SkipVerify {
		v.keyset = newJWKSCache(cfg.IssuerURL, cfg.JWKSCacheTTL, httpc)
	}
	return v, nil
}

// Verify validates rawJWT and returns its projected Identity.
func (v *Verifier) Verify(ctx context.Context, rawJWT string) (*Identity, error) {
	if v.cfg.SkipVerify {
		v.warnSkipVerify()
		return syntheticDevIdentity(), nil
	}

	parser := jwt.NewParser(
		jwt.WithIssuer(v.cfg.IssuerURL),
		jwt.WithLeeway(v.cfg.ClockSkew),
		// We validate audience manually because Keycloak emits `aud` as
		// either a string or an array, and we need to match a single
		// expected audience either way. The library's WithAudience
		// works, but doing it ourselves makes the error path explicit.
		jwt.WithExpirationRequired(),
	)

	tok, err := parser.ParseWithClaims(rawJWT, jwt.MapClaims{}, func(t *jwt.Token) (any, error) {
		// Reject `none` and ensure alg is asymmetric.
		switch t.Method.(type) {
		case *jwt.SigningMethodRSA, *jwt.SigningMethodRSAPSS, *jwt.SigningMethodECDSA:
		default:
			return nil, fmt.Errorf("auth: unexpected signing method %v", t.Header["alg"])
		}
		kid, _ := t.Header["kid"].(string)
		if kid == "" {
			return nil, errors.New("auth: token missing kid")
		}
		return v.keyset.keyForKID(ctx, kid)
	})
	if err != nil {
		return nil, fmt.Errorf("auth: %w", err)
	}
	if !tok.Valid {
		return nil, errors.New("auth: invalid token")
	}

	claims, ok := tok.Claims.(jwt.MapClaims)
	if !ok {
		return nil, errors.New("auth: unexpected claims type")
	}

	if err := checkAudience(claims, v.cfg.Audience); err != nil {
		return nil, err
	}

	return projectIdentity(claims, v.cfg), nil
}

func checkAudience(claims jwt.MapClaims, want string) error {
	raw, ok := claims["aud"]
	if !ok {
		return errors.New("auth: aud claim missing")
	}
	switch a := raw.(type) {
	case string:
		if a == want {
			return nil
		}
	case []any:
		for _, v := range a {
			if s, ok := v.(string); ok && s == want {
				return nil
			}
		}
	case []string:
		for _, s := range a {
			if s == want {
				return nil
			}
		}
	}
	return fmt.Errorf("auth: aud %v does not contain %q", raw, want)
}

func projectIdentity(claims jwt.MapClaims, cfg Config) *Identity {
	id := &Identity{}
	if s, ok := claims["sub"].(string); ok {
		id.Subject = s
	}
	if s, ok := claims["preferred_username"].(string); ok {
		id.PreferredName = s
	}
	if s, ok := claims["email"].(string); ok {
		id.Email = s
	}
	if s, ok := claims["iss"].(string); ok {
		// Keycloak realm is the last path segment of the issuer URL.
		if i := strings.LastIndex(s, "/"); i >= 0 && i < len(s)-1 {
			id.Realm = s[i+1:]
		} else {
			id.Realm = s
		}
	}

	roles := make([]string, 0, 8)
	if ra, ok := claims["realm_access"].(map[string]any); ok {
		roles = appendRoles(roles, ra["roles"])
	}
	if cfg.ResourceClient != "" {
		if res, ok := claims["resource_access"].(map[string]any); ok {
			if c, ok := res[cfg.ResourceClient].(map[string]any); ok {
				roles = appendRoles(roles, c["roles"])
			}
		}
	}
	if cfg.RequiredRolePrefix != "" {
		filtered := roles[:0]
		for _, r := range roles {
			if strings.HasPrefix(r, cfg.RequiredRolePrefix) {
				filtered = append(filtered, r)
			}
		}
		roles = filtered
	}
	id.Roles = roles

	if s, ok := claims["scope"].(string); ok && s != "" {
		id.Scopes = strings.Fields(s)
	}

	if exp, err := claims.GetExpirationTime(); err == nil && exp != nil {
		id.ExpiresAt = exp.Time
	}
	return id
}

func appendRoles(out []string, raw any) []string {
	arr, ok := raw.([]any)
	if !ok {
		return out
	}
	for _, v := range arr {
		if s, ok := v.(string); ok && s != "" {
			out = append(out, s)
		}
	}
	return out
}
