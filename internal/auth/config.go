package auth

import "time"

// Config controls OIDC verification.
type Config struct {
	// IssuerURL is Keycloak's realm URL, e.g.
	// "https://kc.example.com/realms/novanas". OIDC discovery
	// (.well-known/openid-configuration) is fetched from this URL to
	// discover the JWKS endpoint.
	IssuerURL string `json:"issuerUrl"`

	// Audience is the expected `aud` claim. If a token has multiple
	// audiences, this value must appear in the list.
	Audience string `json:"audience"`

	// RequiredRolePrefix optionally filters roles. If set, only roles
	// starting with this prefix are considered (lets operators run
	// mixed-purpose realms). Default "" (all roles).
	RequiredRolePrefix string `json:"requiredRolePrefix,omitempty"`

	// ResourceClient, if non-empty, is the Keycloak client ID whose
	// resource_access.<client>.roles entries should be merged into the
	// Identity's role list alongside realm_access.roles. Empty means
	// realm roles only.
	ResourceClient string `json:"resourceClient,omitempty"`

	// SkipVerify disables JWT validation entirely. ONLY for dev. Logs
	// a loud warning at startup. Default false.
	SkipVerify bool `json:"skipVerify,omitempty"`

	// JWKSCacheTTL is how long to cache the JWKS. Default 1h.
	JWKSCacheTTL time.Duration `json:"jwksCacheTtl,omitempty"`

	// ClockSkew is the allowed clock skew when validating exp/nbf.
	// Default 30s.
	ClockSkew time.Duration `json:"clockSkew,omitempty"`
}

func (c *Config) withDefaults() {
	if c.JWKSCacheTTL <= 0 {
		c.JWKSCacheTTL = time.Hour
	}
	if c.ClockSkew <= 0 {
		c.ClockSkew = 30 * time.Second
	}
}
