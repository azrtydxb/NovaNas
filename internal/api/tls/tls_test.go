package tls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"io"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/novanas/nova-nas/internal/config"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func TestNewReloadableServer_SelfSigned(t *testing.T) {
	dir := t.TempDir()
	cfg := config.TLSConfig{
		HTTPSAddr:          ":0",
		CertDir:            dir,
		MinTLSVersion:      "1.2",
		SelfSignedHostname: "nova-test",
	}
	s, err := NewReloadableServer(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewReloadableServer: %v", err)
	}
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("cert.pem missing: %v", err)
	}
	st, err := os.Stat(keyPath)
	if err != nil {
		t.Fatalf("key.pem missing: %v", err)
	}
	if mode := st.Mode().Perm(); mode != 0o600 {
		t.Errorf("key mode = %o, want 0600", mode)
	}

	pemBytes, err := os.ReadFile(certPath)
	if err != nil {
		t.Fatal(err)
	}
	block, _ := pem.Decode(pemBytes)
	if block == nil {
		t.Fatal("pem decode failed")
	}
	leaf, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}

	hasHostname := false
	for _, n := range leaf.DNSNames {
		if n == "nova-test" {
			hasHostname = true
		}
	}
	if !hasHostname {
		t.Errorf("DNSNames=%v missing nova-test", leaf.DNSNames)
	}
	hasLoopback := false
	for _, ip := range leaf.IPAddresses {
		if ip.Equal(net.ParseIP("127.0.0.1")) {
			hasLoopback = true
		}
	}
	if !hasLoopback {
		t.Errorf("IPAddresses=%v missing 127.0.0.1", leaf.IPAddresses)
	}
	if s.CurrentCertificate() == nil {
		t.Error("current certificate not loaded")
	}
}

func TestNewReloadableServer_OperatorSupplied(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "operator-cert.pem")
	keyPath := filepath.Join(dir, "operator-key.pem")
	writeTestCert(t, certPath, keyPath, "operator-host")

	stCertBefore, _ := os.Stat(certPath)

	cfg := config.TLSConfig{
		HTTPSAddr:     ":0",
		CertPath:      certPath,
		KeyPath:       keyPath,
		MinTLSVersion: "1.2",
	}
	s, err := NewReloadableServer(cfg, testLogger())
	if err != nil {
		t.Fatalf("NewReloadableServer: %v", err)
	}
	if s.CurrentCertificate() == nil {
		t.Fatal("certificate not loaded")
	}
	stCertAfter, _ := os.Stat(certPath)
	if !stCertBefore.ModTime().Equal(stCertAfter.ModTime()) {
		t.Error("operator-supplied cert was rewritten")
	}
	// We must not have created cert.pem in CertDir.
	if _, err := os.Stat(filepath.Join(dir, "cert.pem")); err == nil {
		t.Error("unexpected cert.pem generated alongside operator-supplied cert")
	}
}

func TestReload_HotSwap(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeTestCert(t, certPath, keyPath, "host-a")

	cfg := config.TLSConfig{
		HTTPSAddr:     ":0",
		CertPath:      certPath,
		KeyPath:       keyPath,
		MinTLSVersion: "1.2",
	}
	s, err := NewReloadableServer(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	fpBefore := certFingerprint(s.CurrentCertificate())

	writeTestCert(t, certPath, keyPath, "host-b")
	if err := s.Reload(); err != nil {
		t.Fatalf("reload: %v", err)
	}
	fpAfter := certFingerprint(s.CurrentCertificate())
	if fpBefore == fpAfter {
		t.Fatal("fingerprint did not change after reload")
	}
}

func TestReload_BrokenCertKeepsOld(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeTestCert(t, certPath, keyPath, "host-a")

	s, err := NewReloadableServer(config.TLSConfig{
		HTTPSAddr: ":0", CertPath: certPath, KeyPath: keyPath, MinTLSVersion: "1.2",
	}, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	good := certFingerprint(s.CurrentCertificate())

	// Corrupt the cert file. Reload must error, but keep the old cert.
	if err := os.WriteFile(certPath, []byte("not a cert"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := s.Reload(); err == nil {
		t.Fatal("expected reload error on broken cert")
	}
	if certFingerprint(s.CurrentCertificate()) != good {
		t.Error("broken cert was swapped in; should retain previous cert")
	}
}

func TestRedirectHandler(t *testing.T) {
	s := &ReloadableServer{cfg: config.TLSConfig{HTTPSAddr: ":8443"}, log: testLogger()}
	h := s.redirectHandler()

	req := httptest.NewRequest(http.MethodGet, "http://example.com/api/v1/x?q=1", nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusPermanentRedirect {
		t.Errorf("status=%d want 308", rec.Code)
	}
	loc := rec.Header().Get("Location")
	want := "https://example.com:8443/api/v1/x?q=1"
	if loc != want {
		t.Errorf("Location=%q want %q", loc, want)
	}
}

func TestRedirectHandler_StripsPort(t *testing.T) {
	s := &ReloadableServer{cfg: config.TLSConfig{HTTPSAddr: ":443"}, log: testLogger()}
	h := s.redirectHandler()
	req := httptest.NewRequest(http.MethodGet, "http://example.com:8080/foo", nil)
	req.Host = "example.com:8080"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	loc := rec.Header().Get("Location")
	if loc != "https://example.com/foo" {
		t.Errorf("Location=%q", loc)
	}
}

func TestServe_TLS11Rejected(t *testing.T) {
	dir := t.TempDir()
	cfg := config.TLSConfig{
		HTTPSAddr:          "127.0.0.1:0",
		CertDir:            dir,
		MinTLSVersion:      "1.2",
		SelfSignedHostname: "localhost",
	}
	s, err := NewReloadableServer(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}

	tlsCfg, err := s.TLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	// Client forcing TLS 1.1 should fail handshake.
	conn, err := tls.Dial("tcp", ln.Addr().String(), &tls.Config{
		InsecureSkipVerify: true, //nolint:gosec
		MinVersion:         tls.VersionTLS11,
		MaxVersion:         tls.VersionTLS11,
	})
	if err == nil {
		conn.Close()
		t.Fatal("expected TLS 1.1 handshake to fail, but it succeeded")
	}
}

func TestServe_HotReloadEndToEnd(t *testing.T) {
	dir := t.TempDir()
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	writeTestCert(t, certPath, keyPath, "host-a")

	cfg := config.TLSConfig{
		HTTPSAddr:     "127.0.0.1:0",
		CertPath:      certPath,
		KeyPath:       keyPath,
		MinTLSVersion: "1.2",
	}
	s, err := NewReloadableServer(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	tlsCfg, err := s.TLSConfig()
	if err != nil {
		t.Fatal(err)
	}
	ln, err := tls.Listen("tcp", "127.0.0.1:0", tlsCfg)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {})}
	go srv.Serve(ln) //nolint:errcheck
	defer srv.Close()

	cn1 := dialAndGetCN(t, ln.Addr().String())
	if cn1 != "host-a" {
		t.Fatalf("CN=%q want host-a", cn1)
	}

	writeTestCert(t, certPath, keyPath, "host-b")
	if err := s.Reload(); err != nil {
		t.Fatal(err)
	}
	cn2 := dialAndGetCN(t, ln.Addr().String())
	if cn2 != "host-b" {
		t.Fatalf("CN after reload=%q want host-b", cn2)
	}
}

func TestParseTLSVersion(t *testing.T) {
	cases := map[string]uint16{"": tls.VersionTLS12, "1.2": tls.VersionTLS12, "1.3": tls.VersionTLS13}
	for in, want := range cases {
		got, err := parseTLSVersion(in)
		if err != nil {
			t.Errorf("parse(%q): %v", in, err)
		}
		if got != want {
			t.Errorf("parse(%q)=%x want %x", in, got, want)
		}
	}
	if _, err := parseTLSVersion("1.0"); err == nil {
		t.Error("expected error for TLS 1.0")
	}
}

func TestServe_ContextCancel(t *testing.T) {
	dir := t.TempDir()
	cfg := config.TLSConfig{
		HTTPSAddr:          "127.0.0.1:0",
		CertDir:            dir,
		MinTLSVersion:      "1.2",
		SelfSignedHostname: "localhost",
	}
	s, err := NewReloadableServer(cfg, testLogger())
	if err != nil {
		t.Fatal(err)
	}
	// Use a deliberately-bound port to avoid races; we just check
	// Serve returns when ctx cancels.
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- s.Serve(ctx, http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Serve did not return after ctx cancel")
	}
}

// helpers ----------------------------------------------------------

func dialAndGetCN(t *testing.T, addr string) string {
	t.Helper()
	conn, err := tls.Dial("tcp", addr, &tls.Config{InsecureSkipVerify: true}) //nolint:gosec
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer conn.Close()
	state := conn.ConnectionState()
	if len(state.PeerCertificates) == 0 {
		t.Fatal("no peer certs")
	}
	return state.PeerCertificates[0].Subject.CommonName
}

func writeTestCert(t *testing.T, certPath, keyPath, cn string) {
	t.Helper()
	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: cn, Organization: []string{"NovaNAS"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().Add(24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
		DNSNames:              []string{cn, "localhost"},
		IPAddresses:           []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(certPath, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		t.Fatal(err)
	}
	keyDER, _ := x509.MarshalECPrivateKey(priv)
	if err := os.WriteFile(keyPath, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatal(err)
	}
}

// silence unused-import warnings if a future refactor drops them.
var _ = strings.Contains
