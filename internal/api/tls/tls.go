// Package tls provides an HTTPS listener with hot-reloadable
// certificates and an optional HTTP-to-HTTPS redirect.
//
// On first boot, if the operator supplied no cert paths, a self-signed
// cert is generated under the configured CertDir. The package watches
// the cert/key files (via fsnotify, with a polling fallback) so that
// cert-manager / Let's Encrypt rotations become a no-restart event:
// existing connections finish on their original cert, while new
// handshakes pick up the new one via tls.Config.GetCertificate.
package tls

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/fsnotify/fsnotify"

	"github.com/novanas/nova-nas/internal/config"
)

// pollInterval is the polling fallback when fsnotify is unavailable
// or the watched directory has not yet been created. It also bounds
// reload latency in tests.
const pollInterval = 60 * time.Second

// ReloadableServer wraps a TLS listener whose certificate can be
// swapped without dropping in-flight connections.
type ReloadableServer struct {
	cfg config.TLSConfig
	log *slog.Logger

	current atomic.Pointer[tls.Certificate]

	// fingerprint of the active cert; used to detect actual changes
	// when fsnotify fires for noisy events (e.g. chmod).
	fpMu        sync.Mutex
	fingerprint string
}

// NewReloadableServer prepares the TLS configuration and, if needed,
// generates a self-signed cert at <CertDir>/cert.pem + key.pem.
// It does NOT start listening; call Serve.
func NewReloadableServer(cfg config.TLSConfig, log *slog.Logger) (*ReloadableServer, error) {
	if log == nil {
		log = slog.Default()
	}
	s := &ReloadableServer{cfg: cfg, log: log}

	certPath, keyPath, err := s.resolvePaths()
	if err != nil {
		return nil, err
	}

	// Ensure cert exists. Self-sign if both operator paths were empty
	// and the on-disk files are missing. If the operator pointed us
	// at specific paths, we never fabricate them - that would mask
	// configuration mistakes.
	if cfg.CertPath == "" && cfg.KeyPath == "" {
		if _, statErr := os.Stat(certPath); errors.Is(statErr, os.ErrNotExist) {
			if err := s.generateSelfSigned(certPath, keyPath); err != nil {
				return nil, fmt.Errorf("self-sign: %w", err)
			}
			log.Info("tls: generated self-signed certificate", "cert", certPath, "key", keyPath)
		}
	}

	cert, err := loadCert(certPath, keyPath)
	if err != nil {
		return nil, fmt.Errorf("load cert: %w", err)
	}
	s.current.Store(cert)
	s.setFingerprint(cert)
	return s, nil
}

// Serve starts the HTTPS listener and (when configured) the HTTP
// redirect listener. Blocks until ctx is cancelled.
func (s *ReloadableServer) Serve(ctx context.Context, handler http.Handler) error {
	if s.cfg.HTTPSAddr == "" {
		return errors.New("tls: HTTPSAddr is empty")
	}

	tlsCfg, err := s.tlsConfig()
	if err != nil {
		return err
	}

	httpsSrv := &http.Server{
		Addr:              s.cfg.HTTPSAddr,
		Handler:           handler,
		TLSConfig:         tlsCfg,
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      0, // SSE: see cmd/nova-api/main.go
		IdleTimeout:       60 * time.Second,
	}

	errCh := make(chan error, 2)
	go func() {
		s.log.Info("tls: starting HTTPS listener", "addr", s.cfg.HTTPSAddr)
		if err := httpsSrv.ListenAndServeTLS("", ""); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- fmt.Errorf("https: %w", err)
			return
		}
		errCh <- nil
	}()

	var redirectSrv *http.Server
	if s.cfg.HTTPAddr != "" && !s.cfg.DisableHTTPRedirect {
		redirectSrv = &http.Server{
			Addr:              s.cfg.HTTPAddr,
			Handler:           s.redirectHandler(),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       10 * time.Second,
			WriteTimeout:      10 * time.Second,
			IdleTimeout:       30 * time.Second,
		}
		go func() {
			s.log.Info("tls: starting HTTP redirect listener", "addr", s.cfg.HTTPAddr)
			if err := redirectSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
				errCh <- fmt.Errorf("http redirect: %w", err)
				return
			}
			errCh <- nil
		}()
	}

	// Watcher goroutine. Errors are logged, never returned: a flaky
	// watcher must never tear down the live listener.
	watchCtx, watchCancel := context.WithCancel(ctx)
	defer watchCancel()
	go s.watchLoop(watchCtx)

	select {
	case <-ctx.Done():
	case err := <-errCh:
		if err != nil {
			s.log.Error("tls: listener exited", "err", err)
		}
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpsSrv.Shutdown(shutdownCtx)
	if redirectSrv != nil {
		_ = redirectSrv.Shutdown(shutdownCtx)
	}
	return nil
}

// Reload re-reads the cert/key from disk and atomically swaps them.
// On error the previously-loaded cert is retained: a broken file from
// a half-written rotation must not take the server down.
func (s *ReloadableServer) Reload() error {
	certPath, keyPath, err := s.resolvePaths()
	if err != nil {
		return err
	}
	cert, err := loadCert(certPath, keyPath)
	if err != nil {
		s.log.Error("tls: reload failed; keeping old certificate", "err", err)
		return err
	}
	if !s.fingerprintChanged(cert) {
		return nil
	}
	s.current.Store(cert)
	s.setFingerprint(cert)
	s.log.Info("tls: certificate reloaded", "cert", certPath)
	return nil
}

// CurrentCertificate returns the currently active certificate.
// Exposed for tests and diagnostics.
func (s *ReloadableServer) CurrentCertificate() *tls.Certificate {
	return s.current.Load()
}

// TLSConfig returns the *tls.Config that should be used by callers
// that want to build their own listener (e.g. tests).
func (s *ReloadableServer) TLSConfig() (*tls.Config, error) {
	return s.tlsConfig()
}

func (s *ReloadableServer) tlsConfig() (*tls.Config, error) {
	minVer, err := parseTLSVersion(s.cfg.MinTLSVersion)
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{
		MinVersion: minVer,
		// Deprecated in Go 1.18 and ignored, but harmless and
		// communicates intent for older toolchains.
		PreferServerCipherSuites: true,
		NextProtos:               []string{"h2", "http/1.1"},
		GetCertificate: func(_ *tls.ClientHelloInfo) (*tls.Certificate, error) {
			c := s.current.Load()
			if c == nil {
				return nil, errors.New("tls: no certificate loaded")
			}
			return c, nil
		},
	}
	if len(s.cfg.CipherSuites) > 0 {
		ids, err := parseCipherSuites(s.cfg.CipherSuites)
		if err != nil {
			return nil, err
		}
		cfg.CipherSuites = ids
	}
	return cfg, nil
}

func (s *ReloadableServer) resolvePaths() (cert, key string, err error) {
	if (s.cfg.CertPath == "") != (s.cfg.KeyPath == "") {
		return "", "", errors.New("tls: CertPath and KeyPath must both be set or both empty")
	}
	if s.cfg.CertPath != "" {
		return s.cfg.CertPath, s.cfg.KeyPath, nil
	}
	dir := s.cfg.CertDir
	if dir == "" {
		dir = "/etc/nova-nas/tls"
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", "", fmt.Errorf("mkdir cert dir: %w", err)
	}
	return filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"), nil
}

func (s *ReloadableServer) setFingerprint(c *tls.Certificate) {
	s.fpMu.Lock()
	defer s.fpMu.Unlock()
	s.fingerprint = certFingerprint(c)
}

func (s *ReloadableServer) fingerprintChanged(c *tls.Certificate) bool {
	s.fpMu.Lock()
	defer s.fpMu.Unlock()
	return certFingerprint(c) != s.fingerprint
}

func (s *ReloadableServer) redirectHandler() http.Handler {
	httpsPort := portFromAddr(s.cfg.HTTPSAddr)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		host := r.Host
		if host == "" {
			host = "localhost"
		}
		// Strip port from request host; we control the HTTPS port.
		if h, _, err := net.SplitHostPort(host); err == nil {
			host = h
		}
		target := "https://" + host
		if httpsPort != "" && httpsPort != "443" {
			target += ":" + httpsPort
		}
		target += r.URL.RequestURI()
		http.Redirect(w, r, target, http.StatusPermanentRedirect)
	})
}

func portFromAddr(addr string) string {
	_, port, err := net.SplitHostPort(addr)
	if err != nil {
		return ""
	}
	return port
}

// watchLoop runs fsnotify on the cert directory and falls back to
// polling. Either trigger calls Reload, which is idempotent and
// fingerprint-gated.
func (s *ReloadableServer) watchLoop(ctx context.Context) {
	certPath, _, err := s.resolvePaths()
	if err != nil {
		s.log.Error("tls: watch resolve paths", "err", err)
		return
	}
	dir := filepath.Dir(certPath)

	w, err := fsnotify.NewWatcher()
	if err != nil {
		s.log.Warn("tls: fsnotify unavailable; falling back to polling", "err", err)
		s.pollLoop(ctx)
		return
	}
	defer w.Close()

	if err := w.Add(dir); err != nil {
		s.log.Warn("tls: cannot watch cert dir; falling back to polling", "dir", dir, "err", err)
		s.pollLoop(ctx)
		return
	}

	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case ev, ok := <-w.Events:
			if !ok {
				return
			}
			// cert-manager & friends use atomic rename; that surfaces
			// as Create on the new file. We also catch Write for
			// in-place updates.
			if ev.Op&(fsnotify.Write|fsnotify.Create|fsnotify.Rename) == 0 {
				continue
			}
			if !pathMatches(ev.Name, certPath) {
				continue
			}
			if err := s.Reload(); err != nil {
				s.log.Warn("tls: watch reload error", "err", err)
			}
		case err, ok := <-w.Errors:
			if !ok {
				return
			}
			s.log.Warn("tls: fsnotify error", "err", err)
		case <-ticker.C:
			// Polling backstop: covers NFS-mounted secret dirs,
			// slept laptops, and dropped inotify events.
			if err := s.Reload(); err != nil {
				s.log.Warn("tls: poll reload error", "err", err)
			}
		}
	}
}

func (s *ReloadableServer) pollLoop(ctx context.Context) {
	t := time.NewTicker(pollInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := s.Reload(); err != nil {
				s.log.Warn("tls: poll reload error", "err", err)
			}
		}
	}
}

func pathMatches(a, b string) bool {
	// Allow either the cert or key change to trigger; both live in
	// the same directory. We accept any file under the dir whose
	// basename matches cert.pem or key.pem, or matches the operator
	// supplied paths exactly.
	if a == b {
		return true
	}
	base := filepath.Base(a)
	return base == "cert.pem" || base == "key.pem" ||
		strings.HasSuffix(a, filepath.Base(b))
}

func loadCert(certPath, keyPath string) (*tls.Certificate, error) {
	c, err := tls.LoadX509KeyPair(certPath, keyPath)
	if err != nil {
		return nil, err
	}
	if len(c.Certificate) > 0 {
		leaf, err := x509.ParseCertificate(c.Certificate[0])
		if err == nil {
			c.Leaf = leaf
		}
	}
	return &c, nil
}

func certFingerprint(c *tls.Certificate) string {
	if c == nil || len(c.Certificate) == 0 {
		return ""
	}
	leaf := c.Leaf
	if leaf == nil {
		var err error
		leaf, err = x509.ParseCertificate(c.Certificate[0])
		if err != nil {
			return ""
		}
	}
	return fmt.Sprintf("%x", leaf.Raw)
}

func parseTLSVersion(v string) (uint16, error) {
	switch strings.TrimSpace(v) {
	case "", "1.2":
		return tls.VersionTLS12, nil
	case "1.3":
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("tls: unsupported MinTLSVersion %q (want 1.2 or 1.3)", v)
	}
}

func parseCipherSuites(names []string) ([]uint16, error) {
	all := tls.CipherSuites()
	byName := make(map[string]uint16, len(all))
	for _, cs := range all {
		byName[cs.Name] = cs.ID
	}
	out := make([]uint16, 0, len(names))
	for _, n := range names {
		id, ok := byName[strings.TrimSpace(n)]
		if !ok {
			return nil, fmt.Errorf("tls: unknown cipher suite %q", n)
		}
		out = append(out, id)
	}
	return out, nil
}

// generateSelfSigned writes a fresh ECDSA P-256 self-signed cert+key
// to disk. ECDSA over RSA-4096: faster handshakes, smaller files,
// and equivalent security at the 2026 baseline.
func (s *ReloadableServer) generateSelfSigned(certPath, keyPath string) error {
	hostname := s.cfg.SelfSignedHostname
	if hostname == "" {
		h, err := os.Hostname()
		if err != nil || h == "" {
			h = "localhost"
		}
		hostname = h
	}

	priv, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("ecdsa generate: %w", err)
	}

	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return fmt.Errorf("serial: %w", err)
	}

	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject: pkix.Name{
			CommonName:   hostname,
			Organization: []string{"NovaNAS"},
		},
		Issuer: pkix.Name{
			CommonName:   "NovaNAS Self-Signed",
			Organization: []string{"NovaNAS"},
		},
		NotBefore:             now.Add(-5 * time.Minute),
		NotAfter:              now.Add(365 * 24 * time.Hour),
		KeyUsage:              x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		BasicConstraintsValid: true,
		IsCA:                  true,
	}

	dns := []string{hostname, "localhost"}
	ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
	for _, ip := range interfaceIPs() {
		ips = append(ips, ip)
	}
	tmpl.DNSNames = dedupStrings(dns)
	tmpl.IPAddresses = dedupIPs(ips)

	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &priv.PublicKey, priv)
	if err != nil {
		return fmt.Errorf("create cert: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(certPath), 0o755); err != nil {
		return err
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if err := os.WriteFile(certPath, certPEM, 0o644); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}

	keyDER, err := x509.MarshalECPrivateKey(priv)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: keyDER})
	if err := writeFileMode(keyPath, keyPEM, 0o600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	return nil
}

// writeFileMode writes the file then chmods, in case the umask
// stripped the requested mode bits.
func writeFileMode(path string, data []byte, mode os.FileMode) error {
	if err := os.WriteFile(path, data, mode); err != nil {
		return err
	}
	return os.Chmod(path, mode)
}

func interfaceIPs() []net.IP {
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return nil
	}
	out := make([]net.IP, 0, len(addrs))
	for _, a := range addrs {
		ipNet, ok := a.(*net.IPNet)
		if !ok {
			continue
		}
		if ipNet.IP.IsLoopback() || ipNet.IP.IsUnspecified() {
			continue
		}
		out = append(out, ipNet.IP)
	}
	return out
}

func dedupStrings(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if s == "" {
			continue
		}
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func dedupIPs(in []net.IP) []net.IP {
	seen := make(map[string]struct{}, len(in))
	out := make([]net.IP, 0, len(in))
	for _, ip := range in {
		if ip == nil {
			continue
		}
		k := ip.String()
		if _, ok := seen[k]; ok {
			continue
		}
		seen[k] = struct{}{}
		out = append(out, ip)
	}
	return out
}

// Static check that we're using crypto/rsa import (for tooling that
// might prune it). RSA is available as a fallback if a future
// configuration knob exposes it.
var _ = rsa.PublicKey{}
