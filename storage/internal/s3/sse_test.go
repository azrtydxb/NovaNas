package s3

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestParseSSEHeaders_DecisionTree covers the four SSE modes plus the
// main error cases: unsupported SSE header value, SSE-C without key,
// SSE-C with bad algo, and SSE-C with wrong key length.
func TestParseSSEHeaders_DecisionTree(t *testing.T) {
	goodKey := make([]byte, 32)
	for i := range goodKey {
		goodKey[i] = 0xAB
	}
	goodKeyB64 := base64.StdEncoding.EncodeToString(goodKey)

	cases := []struct {
		name     string
		headers  map[string]string
		wantMode SSEMode
		wantErr  string
	}{
		{"none", map[string]string{}, SSEModeNone, ""},
		{"sse-s3", map[string]string{"x-amz-server-side-encryption": "AES256"}, SSEModeS3, ""},
		{"sse-kms", map[string]string{
			"x-amz-server-side-encryption":              "aws:kms",
			"x-amz-server-side-encryption-aws-kms-key-id": "my-key",
		}, SSEModeKMS, ""},
		{"sse-c", map[string]string{
			"x-amz-server-side-encryption-customer-algorithm": "AES256",
			"x-amz-server-side-encryption-customer-key":       goodKeyB64,
		}, SSEModeC, ""},
		{"sse-bad", map[string]string{"x-amz-server-side-encryption": "DES"}, SSEModeNone, "unsupported"},
		{"sse-c missing key", map[string]string{
			"x-amz-server-side-encryption-customer-algorithm": "AES256",
		}, SSEModeNone, "missing customer-key"},
		{"sse-c bad algo", map[string]string{
			"x-amz-server-side-encryption-customer-algorithm": "DES",
			"x-amz-server-side-encryption-customer-key":       goodKeyB64,
		}, SSEModeNone, "unsupported"},
		{"sse-c short key", map[string]string{
			"x-amz-server-side-encryption-customer-algorithm": "AES256",
			"x-amz-server-side-encryption-customer-key":       base64.StdEncoding.EncodeToString([]byte("short")),
		}, SSEModeNone, "32 bytes"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := http.Header{}
			for k, v := range tc.headers {
				h.Set(k, v)
			}
			got, err := ParseSSEHeaders(h)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil", tc.wantErr)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("expected error %q, got %q", tc.wantErr, err.Error())
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected err: %v", err)
			}
			if got.Mode != tc.wantMode {
				t.Fatalf("mode: want %d got %d", tc.wantMode, got.Mode)
			}
		})
	}
}

// TestGateway_PutObject_SSEKMS exercises the end-to-end wiring:
// SSE-KMS header → RouteSSE → Gateway-level kmsKeyLookup. A missing
// resolver must 501; a resolver that returns a key must allow the PUT.
func TestGateway_PutObject_SSEKMS(t *testing.T) {
	g, bs := newTestGateway()
	seedBucket(t, bs, "sse-bucket")
	payload := []byte("hello kms")

	// 1) Without a resolver configured, SSE-KMS → 501.
	req := httptest.NewRequest(http.MethodPut, "/sse-bucket/o", bytes.NewReader(payload))
	req.Header.Set("x-amz-server-side-encryption", "aws:kms")
	req.Header.Set("x-amz-server-side-encryption-aws-kms-key-id", "arn:novanas:kms:::key/finance")
	setAuthHeaders(req, req.Method, req.URL.Path)
	w := httptest.NewRecorder()
	g.ServeHTTP(w, req)
	if w.Result().StatusCode != http.StatusNotImplemented {
		t.Fatalf("want 501, got %d: %s", w.Result().StatusCode, w.Body.String())
	}

	// 2) With a resolver that hits, PUT succeeds.
	rawDK := bytes.Repeat([]byte{0xFF}, 32)
	g.WithKMSKeyLookup(func(keyID string) ([]byte, bool) {
		if keyID == "arn:novanas:kms:::key/finance" {
			return rawDK, true
		}
		return nil, false
	})
	req2 := httptest.NewRequest(http.MethodPut, "/sse-bucket/o", bytes.NewReader(payload))
	req2.Header.Set("x-amz-server-side-encryption", "aws:kms")
	req2.Header.Set("x-amz-server-side-encryption-aws-kms-key-id", "arn:novanas:kms:::key/finance")
	setAuthHeaders(req2, req2.Method, req2.URL.Path)
	w2 := httptest.NewRecorder()
	g.ServeHTTP(w2, req2)
	if w2.Result().StatusCode/100 != 2 {
		t.Fatalf("want 2xx, got %d: %s", w2.Result().StatusCode, w2.Body.String())
	}

	// 3) Resolver that misses → still 501 for unknown keys.
	req3 := httptest.NewRequest(http.MethodPut, "/sse-bucket/o2", bytes.NewReader(payload))
	req3.Header.Set("x-amz-server-side-encryption", "aws:kms")
	req3.Header.Set("x-amz-server-side-encryption-aws-kms-key-id", "arn:novanas:kms:::key/unknown")
	setAuthHeaders(req3, req3.Method, req3.URL.Path)
	w3 := httptest.NewRecorder()
	g.ServeHTTP(w3, req3)
	if w3.Result().StatusCode != http.StatusNotImplemented {
		t.Fatalf("want 501 for unknown key, got %d", w3.Result().StatusCode)
	}
}

// TestRouteSSE covers the namespace-routing decision: SSE-C -> ssec,
// SSE-KMS without resolver -> error, SSE-KMS with resolver-hit ->
// default namespace, everything else -> default.
func TestRouteSSE(t *testing.T) {
	cases := []struct {
		name    string
		req     *SSERequest
		lookup  func(string) ([]byte, bool)
		wantNS  string
		wantErr bool
	}{
		{"nil -> default", nil, nil, "default", false},
		{"none -> default", &SSERequest{Mode: SSEModeNone}, nil, "default", false},
		{"sse-s3 -> default", &SSERequest{Mode: SSEModeS3}, nil, "default", false},
		{"sse-c -> ssec", &SSERequest{Mode: SSEModeC}, nil, "ssec", false},
		{"sse-kms no resolver -> err", &SSERequest{Mode: SSEModeKMS, KMSKeyID: "k"}, nil, "", true},
		{"sse-kms miss -> err", &SSERequest{Mode: SSEModeKMS, KMSKeyID: "k"}, func(string) ([]byte, bool) { return nil, false }, "", true},
		{"sse-kms hit -> default", &SSERequest{Mode: SSEModeKMS, KMSKeyID: "k"}, func(string) ([]byte, bool) { return []byte("dk"), true }, "default", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ns, err := RouteSSE(tc.req, tc.lookup)
			if tc.wantErr != (err != nil) {
				t.Fatalf("err: want=%v got=%v", tc.wantErr, err)
			}
			if !tc.wantErr && ns != tc.wantNS {
				t.Fatalf("ns: want=%q got=%q", tc.wantNS, ns)
			}
		})
	}
}
