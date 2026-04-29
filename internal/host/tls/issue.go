// Package tls (host/tls) provides a small, in-process X.509 issuer
// backed by the local NovaNAS CA at /etc/nova-ca/{ca.crt,ca.key}.
//
// This package intentionally lives under internal/host (not internal/api/tls
// which deals with the HTTPS listener for nova-api itself) because the
// CA-backed issuance is a host-level facility used by the plugin engine
// to mint short-lived server certs for plugin processes. Keeping it
// separate from the listener-side code keeps the dependency direction
// clean — the plugin engine depends on this package, not on api/tls.
//
// All operations are pure crypto/x509 + crypto/rsa; no shelling out to
// openssl. This makes the helper testable (round-trip CA → child cert
// verify) and avoids depending on `openssl` being present on the host.
package tls

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"time"
)

// DefaultCACertPath is the operator-supplied CA certificate that signs
// plugin server certs. NovaNAS deployments place it here as part of
// host bootstrap (see deploy/observability/issue-certs.sh).
const DefaultCACertPath = "/etc/nova-ca/ca.crt"

// DefaultCAKeyPath is the matching private key.
const DefaultCAKeyPath = "/etc/nova-ca/ca.key"

// Issuer signs leaf server certificates against a parent CA loaded
// from disk. The CA material is re-read per Issue call so on-disk
// rotation is picked up without a restart.
type Issuer struct {
	// CACertPath / CAKeyPath are the on-disk PEM files. Empty falls
	// back to DefaultCACertPath / DefaultCAKeyPath.
	CACertPath string
	CAKeyPath  string

	// Now is overridable in tests; nil → time.Now.
	Now func() time.Time
}

// LeafRequest is the input to Issue. CommonName is required; the rest
// are optional (default TTL is 365 days when zero).
type LeafRequest struct {
	CommonName string
	DNSNames   []string
	IPs        []string // dotted/IPv6 strings
	TTLDays    int
}

// IssuedCert holds the freshly-issued leaf material in PEM form.
type IssuedCert struct {
	CertPEM  []byte
	KeyPEM   []byte
	NotAfter time.Time
}

func (i *Issuer) now() time.Time {
	if i.Now != nil {
		return i.Now()
	}
	return time.Now()
}

func (i *Issuer) caCertPath() string {
	if i.CACertPath != "" {
		return i.CACertPath
	}
	return DefaultCACertPath
}

func (i *Issuer) caKeyPath() string {
	if i.CAKeyPath != "" {
		return i.CAKeyPath
	}
	return DefaultCAKeyPath
}

// loadCA reads + parses the CA cert and key from disk.
func (i *Issuer) loadCA() (*x509.Certificate, *rsa.PrivateKey, error) {
	certBytes, err := os.ReadFile(i.caCertPath())
	if err != nil {
		return nil, nil, fmt.Errorf("tls: read CA cert: %w", err)
	}
	keyBytes, err := os.ReadFile(i.caKeyPath())
	if err != nil {
		return nil, nil, fmt.Errorf("tls: read CA key: %w", err)
	}
	caCert, err := parseCertPEM(certBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("tls: parse CA cert: %w", err)
	}
	caKey, err := parseRSAKeyPEM(keyBytes)
	if err != nil {
		return nil, nil, fmt.Errorf("tls: parse CA key: %w", err)
	}
	return caCert, caKey, nil
}

// Issue mints a new RSA-2048 server-auth certificate for req, signed by
// the configured CA. The returned PEM blocks are independent of the
// Issuer's lifetime — callers may persist them as needed.
func (i *Issuer) Issue(req LeafRequest) (*IssuedCert, error) {
	if req.CommonName == "" {
		return nil, errors.New("tls: CommonName required")
	}
	caCert, caKey, err := i.loadCA()
	if err != nil {
		return nil, err
	}
	leafKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, fmt.Errorf("tls: generate leaf key: %w", err)
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, err
	}
	ttl := req.TTLDays
	if ttl <= 0 {
		ttl = 365
	}
	now := i.now()
	notAfter := now.Add(time.Duration(ttl) * 24 * time.Hour)

	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: req.CommonName},
		NotBefore:    now.Add(-1 * time.Minute),
		NotAfter:     notAfter,
		KeyUsage:     x509.KeyUsageDigitalSignature | x509.KeyUsageKeyEncipherment,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth, x509.ExtKeyUsageClientAuth},
		DNSNames:     append([]string{req.CommonName}, req.DNSNames...),
	}
	for _, s := range req.IPs {
		ip := net.ParseIP(s)
		if ip == nil {
			return nil, fmt.Errorf("tls: invalid IP %q", s)
		}
		tmpl.IPAddresses = append(tmpl.IPAddresses, ip)
	}

	der, err := x509.CreateCertificate(rand.Reader, tmpl, caCert, &leafKey.PublicKey, caKey)
	if err != nil {
		return nil, fmt.Errorf("tls: sign leaf: %w", err)
	}
	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(leafKey)})
	return &IssuedCert{CertPEM: certPEM, KeyPEM: keyPEM, NotAfter: notAfter}, nil
}

// GenerateSelfSignedCA is a test helper that mints a fresh CA with a
// throwaway RSA key. It is exported so tests in other packages can set
// up an ephemeral CA without duplicating crypto boilerplate.
func GenerateSelfSignedCA(commonName string, ttl time.Duration) (caCertPEM, caKeyPEM []byte, err error) {
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		return nil, nil, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return nil, nil, err
	}
	now := time.Now()
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName},
		NotBefore:             now.Add(-1 * time.Minute),
		NotAfter:              now.Add(ttl),
		KeyUsage:              x509.KeyUsageCertSign | x509.KeyUsageCRLSign,
		BasicConstraintsValid: true,
		IsCA:                  true,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return nil, nil, err
	}
	caCertPEM = pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	caKeyPEM = pem.EncodeToMemory(&pem.Block{Type: "RSA PRIVATE KEY", Bytes: x509.MarshalPKCS1PrivateKey(key)})
	return caCertPEM, caKeyPEM, nil
}

// parseCertPEM accepts a single CERTIFICATE block.
func parseCertPEM(b []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

// parseRSAKeyPEM accepts either RSA PRIVATE KEY (PKCS1) or PRIVATE KEY (PKCS8).
func parseRSAKeyPEM(b []byte) (*rsa.PrivateKey, error) {
	block, _ := pem.Decode(b)
	if block == nil {
		return nil, errors.New("no PEM block")
	}
	if k, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return k, nil
	}
	any, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	rk, ok := any.(*rsa.PrivateKey)
	if !ok {
		return nil, errors.New("CA key is not RSA")
	}
	return rk, nil
}
