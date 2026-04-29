package auth

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// testIdP spins up an httptest server that serves OIDC discovery and a
// JWKS document for a single RSA key. It exposes mintToken to forge
// tokens with arbitrary claims.
type testIdP struct {
	srv      *httptest.Server
	priv     *rsa.PrivateKey
	kid      string
	issuer   string
	audience string
}

func newTestIdP(t *testing.T) *testIdP {
	t.Helper()
	priv, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa keygen: %v", err)
	}
	idp := &testIdP{priv: priv, kid: "test-kid-1", audience: "novanas"}

	mux := http.NewServeMux()
	mux.HandleFunc("/.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"issuer":   idp.issuer,
			"jwks_uri": idp.issuer + "/jwks",
		})
	})
	mux.HandleFunc("/jwks", func(w http.ResponseWriter, r *http.Request) {
		n := base64.RawURLEncoding.EncodeToString(priv.N.Bytes())
		// e=65537 → big-endian 0x010001
		eBytes := []byte{0x01, 0x00, 0x01}
		e := base64.RawURLEncoding.EncodeToString(eBytes)
		_ = json.NewEncoder(w).Encode(map[string]any{
			"keys": []map[string]any{{
				"kty": "RSA",
				"kid": idp.kid,
				"alg": "RS256",
				"use": "sig",
				"n":   n,
				"e":   e,
			}},
		})
	})
	idp.srv = httptest.NewServer(mux)
	idp.issuer = idp.srv.URL
	t.Cleanup(idp.srv.Close)
	return idp
}

func (i *testIdP) mintToken(t *testing.T, claims jwt.MapClaims) string {
	t.Helper()
	if _, ok := claims["iss"]; !ok {
		claims["iss"] = i.issuer
	}
	if _, ok := claims["aud"]; !ok {
		claims["aud"] = i.audience
	}
	if _, ok := claims["exp"]; !ok {
		claims["exp"] = time.Now().Add(5 * time.Minute).Unix()
	}
	tok := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	tok.Header["kid"] = i.kid
	s, err := tok.SignedString(i.priv)
	if err != nil {
		t.Fatalf("sign: %v", err)
	}
	return s
}

func newTestVerifier(t *testing.T, idp *testIdP) *Verifier {
	t.Helper()
	v, err := NewVerifier(Config{
		IssuerURL:    idp.issuer,
		Audience:     idp.audience,
		JWKSCacheTTL: time.Minute,
		ClockSkew:    30 * time.Second,
	}, idp.srv.Client())
	if err != nil {
		t.Fatalf("NewVerifier: %v", err)
	}
	return v
}

func TestVerifyHappyPath(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	tok := idp.mintToken(t, jwt.MapClaims{
		"sub":                "user-1",
		"preferred_username": "alice",
		"email":              "alice@example.com",
		"realm_access":       map[string]any{"roles": []any{"nova-admin", "default-roles-x"}},
		"scope":              "openid profile",
	})
	id, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if id.Subject != "user-1" || id.PreferredName != "alice" || id.Email != "alice@example.com" {
		t.Errorf("identity fields wrong: %+v", id)
	}
	if !contains(id.Roles, "nova-admin") {
		t.Errorf("missing nova-admin: %v", id.Roles)
	}
	if !contains(id.Scopes, "openid") || !contains(id.Scopes, "profile") {
		t.Errorf("scopes wrong: %v", id.Scopes)
	}
}

func TestVerifyBadSignature(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	tok := idp.mintToken(t, jwt.MapClaims{"sub": "u"})
	// Flip the last char of the signature.
	parts := strings.Split(tok, ".")
	if len(parts) != 3 {
		t.Fatalf("bad token shape")
	}
	// Replace the first char of the signature with a different base64 char.
	first := parts[2][0]
	repl := byte('A')
	if first == 'A' {
		repl = 'B'
	}
	parts[2] = string(repl) + parts[2][1:]
	tampered := strings.Join(parts, ".")

	if _, err := v.Verify(context.Background(), tampered); err == nil {
		t.Fatalf("expected error, got nil")
	}
}

func TestVerifyBadAudience(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	tok := idp.mintToken(t, jwt.MapClaims{"sub": "u", "aud": "wrong-audience"})
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatalf("expected aud rejection")
	}
}

func TestVerifyAudienceArray(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	tok := idp.mintToken(t, jwt.MapClaims{
		"sub": "u",
		"aud": []string{"other", idp.audience},
	})
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("aud array: %v", err)
	}
}

func TestVerifyExpired(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	// Expired beyond clock skew tolerance.
	tok := idp.mintToken(t, jwt.MapClaims{
		"sub": "u",
		"exp": time.Now().Add(-5 * time.Minute).Unix(),
	})
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatalf("expected expired rejection")
	}
}

func TestVerifyClockSkewTolerance(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	// Just-expired but within skew (30s default).
	tok := idp.mintToken(t, jwt.MapClaims{
		"sub": "u",
		"exp": time.Now().Add(-5 * time.Second).Unix(),
	})
	if _, err := v.Verify(context.Background(), tok); err != nil {
		t.Fatalf("should tolerate small skew: %v", err)
	}
}

func TestVerifyBadIssuer(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	tok := idp.mintToken(t, jwt.MapClaims{"sub": "u", "iss": "https://evil.example.com/realms/x"})
	if _, err := v.Verify(context.Background(), tok); err == nil {
		t.Fatalf("expected issuer rejection")
	}
}

func TestRolePrefixFilter(t *testing.T) {
	idp := newTestIdP(t)
	v, err := NewVerifier(Config{
		IssuerURL:          idp.issuer,
		Audience:           idp.audience,
		RequiredRolePrefix: "nova-",
	}, idp.srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	tok := idp.mintToken(t, jwt.MapClaims{
		"sub": "u",
		"realm_access": map[string]any{
			"roles": []any{"nova-admin", "default-roles-x", "uma_authorization"},
		},
	})
	id, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if len(id.Roles) != 1 || id.Roles[0] != "nova-admin" {
		t.Errorf("expected only nova-admin, got %v", id.Roles)
	}
}

func TestResourceAccessRoles(t *testing.T) {
	idp := newTestIdP(t)
	v, err := NewVerifier(Config{
		IssuerURL:      idp.issuer,
		Audience:       idp.audience,
		ResourceClient: "novanas-api",
	}, idp.srv.Client())
	if err != nil {
		t.Fatal(err)
	}

	tok := idp.mintToken(t, jwt.MapClaims{
		"sub":          "u",
		"realm_access": map[string]any{"roles": []any{"nova-viewer"}},
		"resource_access": map[string]any{
			"novanas-api": map[string]any{"roles": []any{"nova-operator"}},
			"other":       map[string]any{"roles": []any{"ignored"}},
		},
	})
	id, err := v.Verify(context.Background(), tok)
	if err != nil {
		t.Fatal(err)
	}
	if !contains(id.Roles, "nova-viewer") || !contains(id.Roles, "nova-operator") {
		t.Errorf("missing merged roles: %v", id.Roles)
	}
	if contains(id.Roles, "ignored") {
		t.Errorf("leaked role from other client: %v", id.Roles)
	}
}

func TestRBAC(t *testing.T) {
	id := &Identity{Roles: []string{"nova-viewer"}}
	if !IdentityHasPermission(DefaultRoleMap, id, PermStorageRead) {
		t.Errorf("viewer should have storage:read")
	}
	if IdentityHasPermission(DefaultRoleMap, id, PermStorageWrite) {
		t.Errorf("viewer should NOT have storage:write")
	}
	admin := &Identity{Roles: []string{"nova-admin"}}
	if !IdentityHasPermission(DefaultRoleMap, admin, PermSystemAdmin) {
		t.Errorf("admin should have system:admin")
	}
	if IdentityHasPermission(DefaultRoleMap, nil, PermStorageRead) {
		t.Errorf("nil identity granted permission")
	}
}

func TestMiddlewareBypassesPublicPaths(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	mw := v.Middleware([]string{"/healthz", "/metrics"}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	called := false
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if !called || rr.Code != http.StatusOK {
		t.Errorf("public path blocked: called=%v code=%d", called, rr.Code)
	}
}

func TestMiddlewareRejectsMissingToken(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	mw := v.Middleware(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatalf("handler should not be reached")
	}))

	req := httptest.NewRequest(http.MethodGet, "/storage/pools", nil)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", rr.Code)
	}
}

func TestMiddlewareAttachesIdentity(t *testing.T) {
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	tok := idp.mintToken(t, jwt.MapClaims{
		"sub":          "u",
		"realm_access": map[string]any{"roles": []any{"nova-admin"}},
	})

	mw := v.Middleware(nil, slog.New(slog.NewTextHandler(io.Discard, nil)))
	var got *Identity
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id, ok := IdentityFromContext(r.Context())
		if !ok {
			t.Fatalf("no identity")
		}
		got = id
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Authorization", "Bearer "+tok)
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("want 200, got %d body=%s", rr.Code, rr.Body.String())
	}
	if got == nil || got.Subject != "u" {
		t.Errorf("identity not stashed: %+v", got)
	}
}

func TestRequirePermission(t *testing.T) {
	mw := RequirePermission(DefaultRoleMap, PermStorageWrite)
	h := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Viewer rejected.
	req := httptest.NewRequest(http.MethodPost, "/x", nil)
	req = req.WithContext(WithIdentity(req.Context(), &Identity{Roles: []string{"nova-viewer"}}))
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusForbidden {
		t.Errorf("viewer should be 403, got %d", rr.Code)
	}

	// Operator accepted.
	req = httptest.NewRequest(http.MethodPost, "/x", nil)
	req = req.WithContext(WithIdentity(req.Context(), &Identity{Roles: []string{"nova-operator"}}))
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("operator should be 200, got %d", rr.Code)
	}

	// No identity → 401.
	req = httptest.NewRequest(http.MethodPost, "/x", nil)
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("no-identity should be 401, got %d", rr.Code)
	}
}

func TestSkipVerifyAcceptsAnything(t *testing.T) {
	var buf bytes.Buffer
	logger := slog.New(slog.NewTextHandler(&buf, nil))
	SetDevLogger(logger)
	t.Cleanup(func() { SetDevLogger(nil); devWarnLast.Store(0) })
	devWarnLast.Store(0)

	v, err := NewVerifier(Config{SkipVerify: true}, nil)
	if err != nil {
		t.Fatal(err)
	}
	id, err := v.Verify(context.Background(), "literally-anything")
	if err != nil {
		t.Fatalf("SkipVerify should accept: %v", err)
	}
	if !contains(id.Roles, "nova-admin") {
		t.Errorf("synthetic identity missing nova-admin: %v", id.Roles)
	}
	if !strings.Contains(buf.String(), "SKIP VERIFY") {
		t.Errorf("warning not logged: %q", buf.String())
	}

	// Second call within rate-limit should not log again.
	buf.Reset()
	if _, err := v.Verify(context.Background(), "x"); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buf.String(), "SKIP VERIFY") {
		t.Errorf("warning should be rate-limited: %q", buf.String())
	}
}

func TestJWKSKidRotation(t *testing.T) {
	// First call populates cache. Then rotate the kid on the IdP and
	// verify a token signed with the new kid: the cache should refresh
	// on the miss.
	idp := newTestIdP(t)
	v := newTestVerifier(t, idp)

	tok1 := idp.mintToken(t, jwt.MapClaims{"sub": "u1"})
	if _, err := v.Verify(context.Background(), tok1); err != nil {
		t.Fatal(err)
	}

	idp.kid = "test-kid-2"
	tok2 := idp.mintToken(t, jwt.MapClaims{"sub": "u2"})
	if _, err := v.Verify(context.Background(), tok2); err != nil {
		t.Fatalf("rotation: %v", err)
	}
}

func TestParseBearer(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r.Header.Set("Authorization", "bearer abc.def.ghi")
	tok, ok := bearerToken(r)
	if !ok || tok != "abc.def.ghi" {
		t.Errorf("got %q ok=%v", tok, ok)
	}
	r.Header.Set("Authorization", "Basic xxx")
	if _, ok := bearerToken(r); ok {
		t.Errorf("non-bearer accepted")
	}
}

func contains(ss []string, s string) bool {
	for _, x := range ss {
		if x == s {
			return true
		}
	}
	return false
}

// silence unused import lint for url in older builds
var _ = url.Parse
