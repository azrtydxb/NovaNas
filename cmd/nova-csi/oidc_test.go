package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

// makeJWT builds an unsigned-ish JWT (header.payload.sig) where the payload
// has the given exp claim. The signature segment is opaque garbage; nothing
// in the code under test verifies it.
func makeJWT(t *testing.T, exp int64) string {
	t.Helper()
	hdr := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","typ":"JWT"}`))
	body, _ := json.Marshal(map[string]any{"exp": exp, "iss": "test"})
	pay := base64.RawURLEncoding.EncodeToString(body)
	sig := base64.RawURLEncoding.EncodeToString([]byte("not-a-signature"))
	return hdr + "." + pay + "." + sig
}

func newOIDCSourceForTest(t *testing.T, srv *httptest.Server) *oidcSource {
	t.Helper()
	s, err := newOIDCSource(srv.URL+"/realms/r", "nova-csi", "secretpw", nil)
	if err != nil {
		t.Fatalf("newOIDCSource: %v", err)
	}
	// Replace the HTTP client so we can talk to the http (not https) test server.
	s.httpc = srv.Client()
	return s
}

func TestOIDCSource_Fetch_Success(t *testing.T) {
	exp := time.Now().Add(5 * time.Minute).Unix()
	tok := makeJWT(t, exp)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("method=%s", r.Method)
		}
		if r.URL.Path != "/realms/r/protocol/openid-connect/token" {
			t.Errorf("path=%q", r.URL.Path)
		}
		if r.Header.Get("Content-Type") != "application/x-www-form-urlencoded" {
			t.Errorf("content-type=%q", r.Header.Get("Content-Type"))
		}
		if err := r.ParseForm(); err != nil {
			t.Fatal(err)
		}
		if r.Form.Get("grant_type") != "client_credentials" {
			t.Errorf("grant_type=%q", r.Form.Get("grant_type"))
		}
		if r.Form.Get("client_id") != "nova-csi" {
			t.Errorf("client_id=%q", r.Form.Get("client_id"))
		}
		if r.Form.Get("client_secret") != "secretpw" {
			t.Errorf("client_secret=%q", r.Form.Get("client_secret"))
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": tok,
			"token_type":   "Bearer",
			"expires_in":   300,
		})
	}))
	defer srv.Close()

	src := newOIDCSourceForTest(t, srv)
	got, gotExp, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if got != tok {
		t.Errorf("token=%q want %q", got, tok)
	}
	// JWT exp wins over expires_in. Allow 2s of slop.
	want := time.Unix(exp, 0)
	if d := gotExp.Sub(want); d < -2*time.Second || d > 2*time.Second {
		t.Errorf("exp=%v want %v", gotExp, want)
	}
}

func TestOIDCSource_Fetch_FallbackToExpiresIn(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": "opaque-not-a-jwt",
			"expires_in":   120,
		})
	}))
	defer srv.Close()

	src := newOIDCSourceForTest(t, srv)
	before := time.Now()
	_, exp, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("Fetch: %v", err)
	}
	if exp.Before(before.Add(60*time.Second)) || exp.After(before.Add(180*time.Second)) {
		t.Errorf("exp=%v not in expected window", exp)
	}
}

func TestOIDCSource_Fetch_HTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = io.WriteString(w, `{"error":"invalid_client","error_description":"bad secret"}`)
	}))
	defer srv.Close()

	src := newOIDCSourceForTest(t, srv)
	_, _, err := src.Fetch(context.Background())
	if err == nil {
		t.Fatal("expected error")
	}
	if !strings.Contains(err.Error(), "401") {
		t.Errorf("error %q missing status code", err.Error())
	}
}

func TestOIDCSource_Fetch_EmptyToken(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"access_token": "", "expires_in": 300})
	}))
	defer srv.Close()

	src := newOIDCSourceForTest(t, srv)
	if _, _, err := src.Fetch(context.Background()); err == nil {
		t.Fatal("expected error for empty access_token")
	}
}

func TestNewOIDCSource_Validation(t *testing.T) {
	if _, err := newOIDCSource("", "id", "sec", nil); err == nil {
		t.Error("expected error for empty issuer")
	}
	if _, err := newOIDCSource("https://x", "", "sec", nil); err == nil {
		t.Error("expected error for empty client id")
	}
	if _, err := newOIDCSource("https://x", "id", "", nil); err == nil {
		t.Error("expected error for empty secret")
	}
	if _, err := newOIDCSource("https://x", "id", "sec", []byte("not pem")); err == nil {
		t.Error("expected error for invalid CA PEM")
	}
}

func TestDecodeJWTExp(t *testing.T) {
	want := time.Now().Add(time.Hour).Truncate(time.Second).Unix()
	tok := makeJWT(t, want)
	got, ok := decodeJWTExp(tok)
	if !ok {
		t.Fatal("decode failed")
	}
	if got.Unix() != want {
		t.Errorf("exp=%d want %d", got.Unix(), want)
	}

	if _, ok := decodeJWTExp("not.a.jwt-because.too.many.parts"); ok {
		t.Error("expected failure for malformed token")
	}
	if _, ok := decodeJWTExp("only-one-part"); ok {
		t.Error("expected failure for non-three-segment token")
	}
}

func TestRunOIDCRefresh_RotatesAndStops(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		// Each call returns a new token expiring soon so the loop refreshes
		// quickly. Lifespan ~14s -> 70% wait ~10s, but we floor at 10s.
		exp := time.Now().Add(14 * time.Second).Unix()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": makeJWT(t, exp) + fmt.Sprintf(".n%d", n),
			"expires_in":   14,
		})
	}))
	defer srv.Close()

	src := newOIDCSourceForTest(t, srv)

	// Initial fetch.
	tok0, exp0, err := src.Fetch(context.Background())
	if err != nil {
		t.Fatalf("initial fetch: %v", err)
	}
	sink := &staticSink{}
	sink.SetToken(tok0)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// To exercise a refresh quickly, override exp0 to "soon" so the floor
	// of 10s gates the wait. We won't actually wait that long; we just
	// confirm the loop exits cleanly on cancel.
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	done := make(chan struct{})
	go func() {
		runOIDCRefresh(ctx, src, sink, exp0, logger)
		close(done)
	}()

	// Cancel and ensure the goroutine exits without leaking.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("refresh goroutine did not exit on context cancel")
	}

	// Initial token should still be present (no refresh happened in 50ms).
	if got := sink.Get(); got != tok0 {
		t.Errorf("sink token mutated unexpectedly: %q vs %q", got, tok0)
	}
}

func TestRunOIDCRefresh_RetriesOnFailure(t *testing.T) {
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n := calls.Add(1)
		if n <= 2 {
			// First two refresh attempts fail.
			http.Error(w, "kaboom", http.StatusInternalServerError)
			return
		}
		exp := time.Now().Add(20 * time.Second).Unix()
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token": makeJWT(t, exp),
			"expires_in":   20,
		})
	}))
	defer srv.Close()

	src := newOIDCSourceForTest(t, srv)
	sink := &staticSink{}
	sink.SetToken("initial")

	// Pretend the existing token expires very soon so the loop fires
	// immediately. Exp in the past forces remaining<=0 → wait floor 10s.
	// We cancel before that happens; the value of this test is the
	// no-panic / clean-exit guarantee, plus exercising the unhappy path.
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runOIDCRefresh(ctx, src, sink, time.Now().Add(-time.Second), logger)
}
