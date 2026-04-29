// Package auth implements OIDC JWT verification and RBAC for NovaNAS as
// a Keycloak resource server.
//
// NovaNAS does not own identities or store users/tokens. It validates
// signed JWTs issued by an external Keycloak realm against the realm's
// JWKS, then maps Keycloak realm-roles onto a small set of NovaNAS
// Permissions via a configurable RoleMap.
//
// Typical wiring:
//
//	v, _ := auth.NewVerifier(cfg, http.DefaultClient)
//	r.Use(v.Middleware([]string{"/healthz", "/metrics"}, log))
//	r.With(auth.RequirePermission(auth.DefaultRoleMap, auth.PermStorageWrite)).
//	    Post("/storage/pools", ...)
//
// # Dev mode
//
// Setting Config.SkipVerify = true short-circuits verification and
// returns a synthetic admin Identity for every request. This is DEV
// ONLY and emits a rate-limited (one per minute) loud warning. Never
// enable in production.
package auth
