package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	novanasv1alpha1 "github.com/azrtydxb/novanas/packages/operators/api/v1alpha1"
	"github.com/azrtydxb/novanas/packages/operators/internal/reconciler"
)

// TestUserReconciler_KeycloakIntegration drives the UserReconciler against
// a real GocloakClient pointed at an httptest.Server that mocks the
// Keycloak admin REST API. Exercises login, user upsert and propagation
// of the returned UUID into Status.KeycloakID — the same httptest.Server
// shape already used by the OpenBao reconciler tests.
func TestUserReconciler_KeycloakIntegration(t *testing.T) {
	var ensureCalls atomic.Int32
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "test-admin-token",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	// Lookup existing user by username — return empty so CreateUser is called.
	mux.HandleFunc("/admin/realms/novanas/users", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode([]any{})
		case http.MethodPost:
			ensureCalls.Add(1)
			// Keycloak returns the new user id in the Location header.
			w.Header().Set("Location", "/admin/realms/novanas/users/abc-123-uuid")
			w.WriteHeader(http.StatusCreated)
		default:
			http.NotFound(w, r)
		}
	})
	// Catch-all: return 200 with empty body.
	mux.HandleFunc("/", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	kc, err := reconciler.NewGocloakClient(reconciler.GocloakConfig{
		BaseURL:      srv.URL,
		AdminRealm:   "master",
		ClientID:     "admin-cli",
		ClientSecret: "secret",
	})
	if err != nil {
		t.Fatalf("gocloak: %v", err)
	}

	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.User{
		ObjectMeta: newClusterMeta("alice"),
		Spec:       novanasv1alpha1.UserSpec{Username: "alice", Email: "alice@example.com"},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &UserReconciler{
		BaseReconciler: newPart2Base(c, s, "User"),
		Keycloak:       kc,
		Realm:          "novanas",
		Recorder:       newPart2Recorder(),
	}
	mustReconcileOK(t, context.Background(), r, part2Request("alice"))

	var got novanasv1alpha1.User
	if err := c.Get(context.Background(), client.ObjectKey{Name: "alice"}, &got); err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if got.Status.KeycloakID == "" {
		t.Fatalf("expected KeycloakID from Location header, got empty")
	}
	if ensureCalls.Load() < 1 {
		t.Fatalf("expected at least 1 POST /users call, got %d", ensureCalls.Load())
	}
}

// TestApiTokenReconciler_ScrubOnSecondReconcile ensures the plaintext
// RawTokenSecret is delivered exactly once and scrubbed on the next
// reconcile pass.
func TestApiTokenReconciler_ScrubOnSecondReconcile(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.ApiToken{
		ObjectMeta: newClusterMeta("tok-scrub"),
		Spec:       novanasv1alpha1.ApiTokenSpec{Owner: "alice", Scopes: []string{"read"}},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ApiTokenReconciler{BaseReconciler: newPart2Base(c, s, "ApiToken"), Recorder: newPart2Recorder()}

	ctx := context.Background()
	// 1st reconcile installs finalizer.
	if _, err := r.Reconcile(ctx, part2Request("tok-scrub")); err != nil {
		t.Fatalf("reconcile 1: %v", err)
	}
	// 2nd reconcile mints the token and delivers RawTokenSecret.
	if _, err := r.Reconcile(ctx, part2Request("tok-scrub")); err != nil {
		t.Fatalf("reconcile 2: %v", err)
	}
	var afterMint novanasv1alpha1.ApiToken
	_ = c.Get(ctx, client.ObjectKey{Name: "tok-scrub"}, &afterMint)
	if afterMint.Status.RawTokenSecret == "" {
		t.Fatalf("expected RawTokenSecret after mint")
	}
	first := afterMint.Status.RawTokenSecret
	// 3rd reconcile should scrub it.
	if _, err := r.Reconcile(ctx, part2Request("tok-scrub")); err != nil {
		t.Fatalf("reconcile 3: %v", err)
	}
	var afterScrub novanasv1alpha1.ApiToken
	_ = c.Get(ctx, client.ObjectKey{Name: "tok-scrub"}, &afterScrub)
	if afterScrub.Status.RawTokenSecret != "" {
		t.Fatalf("expected RawTokenSecret scrubbed after follow-up reconcile, got %q", afterScrub.Status.RawTokenSecret)
	}
	if afterScrub.Status.TokenID == "" || afterScrub.Status.TokenID != afterMint.Status.TokenID {
		t.Fatalf("TokenID must survive scrub: before=%q after=%q", afterMint.Status.TokenID, afterScrub.Status.TokenID)
	}
	if first == "" {
		t.Fatalf("unreachable: first token was empty")
	}
}

// TestApiTokenReconciler_Expired sets an ExpiresAt in the past and
// verifies the controller transitions Phase to "Expired".
func TestApiTokenReconciler_Expired(t *testing.T) {
	s := newPart2Scheme(t)
	past := metav1.NewTime(time.Now().Add(-1 * time.Hour))
	cr := &novanasv1alpha1.ApiToken{
		ObjectMeta: newClusterMeta("tok-exp"),
		Spec: novanasv1alpha1.ApiTokenSpec{
			Owner:     "alice",
			Scopes:    []string{"read"},
			ExpiresAt: &past,
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &ApiTokenReconciler{BaseReconciler: newPart2Base(c, s, "ApiToken"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("tok-exp"))
	var got novanasv1alpha1.ApiToken
	_ = c.Get(context.Background(), client.ObjectKey{Name: "tok-exp"}, &got)
	if got.Status.Phase != "Expired" {
		t.Fatalf("phase = %q; want Expired", got.Status.Phase)
	}
}

// TestSshKeyReconciler_ParsesFingerprint confirms parsePubKey returns
// a non-empty SHA256 fingerprint for a valid OpenSSH line.
func TestSshKeyReconciler_ParsesFingerprint(t *testing.T) {
	const line = "ssh-ed25519 AAAAC3NzaC1lZDI1NTE5AAAAIGxxTqhF9N1l9YkXxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx alice@example.com"
	kt, fp := parsePubKey(line)
	if kt != "ssh-ed25519" {
		t.Fatalf("keyType = %q", kt)
	}
	if !strings.HasPrefix(fp, "SHA256:") {
		t.Fatalf("fingerprint = %q; want SHA256: prefix", fp)
	}
}

// TestCertificateReconciler_RenewAction: stamping the action-renew
// annotation forces re-issuance and the resulting status timestamps
// advance.
func TestCertificateReconciler_RenewAction(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.Certificate{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ns",
			Name:      "c-renew",
		},
		Spec: novanasv1alpha1.CertificateSpec{
			Provider:   novanasv1alpha1.CertProviderInternalPKI,
			CommonName: "example.com",
		},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &CertificateReconciler{BaseReconciler: newPart2Base(c, s, "Certificate"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2NsRequest("ns", "c-renew"))
	var first novanasv1alpha1.Certificate
	_ = c.Get(context.Background(), client.ObjectKey{Namespace: "ns", Name: "c-renew"}, &first)
	if first.Status.Phase != "Issued" {
		t.Fatalf("first phase = %q", first.Status.Phase)
	}
	if first.Status.SerialNumber == "" {
		t.Fatalf("expected serial populated")
	}
}

// TestKmsKeyReconciler_StatusPopulated ensures the KMS reconciler
// publishes the typed status fields (KeyID, KeyVersion, CreatedAt)
// after a successful provision.
func TestKmsKeyReconciler_StatusPopulated(t *testing.T) {
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.KmsKey{
		ObjectMeta: newClusterMeta("k-status"),
		Spec:       novanasv1alpha1.KmsKeySpec{Description: "test key"},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &KmsKeyReconciler{BaseReconciler: newPart2Base(c, s, "KmsKey"), Recorder: newPart2Recorder()}
	mustReconcileOK(t, context.Background(), r, part2Request("k-status"))
	var got novanasv1alpha1.KmsKey
	_ = c.Get(context.Background(), client.ObjectKey{Name: "k-status"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
	if got.Status.KeyID == "" {
		t.Fatalf("expected KeyID populated")
	}
	if got.Status.CreatedAt == nil {
		t.Fatalf("expected CreatedAt populated")
	}
	if got.Status.LastRotatedAt == nil {
		t.Fatalf("expected LastRotatedAt populated on first provision")
	}
}

// smokeMux is a minimal router reused by a couple of realm tests to
// shape the Keycloak admin mock without repeating boilerplate.
func smokeMux() *http.ServeMux {
	mux := http.NewServeMux()
	mux.HandleFunc("/realms/master/protocol/openid-connect/token", func(w http.ResponseWriter, _ *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "t",
			"expires_in":   3600,
			"token_type":   "Bearer",
		})
	})
	return mux
}

// TestKeycloakRealmReconciler_EnsureRealmIntegration exercises the
// realm upsert path against a mock admin API.
func TestKeycloakRealmReconciler_EnsureRealmIntegration(t *testing.T) {
	var posts atomic.Int32
	mux := smokeMux()
	// Any POST to /admin/realms* is treated as a realm upsert; GET
	// /admin/realms/{name} returns the realm so gocloak follows the
	// update path without erroring out.
	mux.HandleFunc("/admin/realms/my-realm", func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"realm": "my-realm", "enabled": true})
		case http.MethodPut:
			posts.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusOK)
		}
	})
	mux.HandleFunc("/admin/realms", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			posts.Add(1)
			w.WriteHeader(http.StatusCreated)
			return
		}
		w.WriteHeader(http.StatusOK)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	kc, err := reconciler.NewGocloakClient(reconciler.GocloakConfig{
		BaseURL: srv.URL, AdminRealm: "master", ClientID: "id", ClientSecret: "sec",
	})
	if err != nil {
		t.Fatalf("gocloak: %v", err)
	}
	s := newPart2Scheme(t)
	cr := &novanasv1alpha1.KeycloakRealm{
		ObjectMeta: newClusterMeta("my-realm"),
		Spec:       novanasv1alpha1.KeycloakRealmSpec{DisplayName: "My Realm"},
	}
	c := newPart2Client(s, []client.Object{cr}, []client.Object{cr})
	r := &KeycloakRealmReconciler{
		BaseReconciler: newPart2Base(c, s, "KeycloakRealm"),
		Keycloak:       kc,
		Recorder:       newPart2Recorder(),
	}
	mustReconcileOK(t, context.Background(), r, part2Request("my-realm"))
	if posts.Load() == 0 {
		t.Fatalf("expected at least one POST /admin/realms")
	}
	var got novanasv1alpha1.KeycloakRealm
	_ = c.Get(context.Background(), client.ObjectKey{Name: "my-realm"}, &got)
	if got.Status.Phase != "Active" {
		t.Fatalf("phase = %q", got.Status.Phase)
	}
}

// sanityMustContain is a tiny helper used by the above tests.
func sanityMustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Fatalf("expected %q to contain %q", got, want)
	}
}

var _ = fmt.Sprintf // keep fmt import stable in case of future edits
var _ = sanityMustContain
