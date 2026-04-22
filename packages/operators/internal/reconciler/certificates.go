package reconciler

import (
	"context"
	"time"
)

// CertificateRequest is the minimal payload a controller passes to a
// CertificateIssuer.
type CertificateRequest struct {
	Name       string
	CommonName string
	DNSNames   []string
	IPSANs     []string
	// Duration requested; issuer may override.
	Duration time.Duration
	// IssuerRef names the upstream issuer (cert-manager Issuer, internal CA,
	// etc.). Implementations are free to ignore this.
	IssuerRef string
}

// CertificateBundle is the returned certificate material.
type CertificateBundle struct {
	CertPEM   []byte
	KeyPEM    []byte
	CAPEM     []byte
	NotBefore time.Time
	NotAfter  time.Time
	Serial    string
}

// CertificateIssuer abstracts the X.509 issuance backend. Real
// implementations talk to cert-manager or an internal PKI; tests use
// NoopCertificateIssuer.
type CertificateIssuer interface {
	Issue(ctx context.Context, req CertificateRequest) (CertificateBundle, error)
	Revoke(ctx context.Context, serial string) error
}

// NoopCertificateIssuer returns a deterministic placeholder bundle. It is
// intentionally NOT a real certificate -- controllers that depend on the
// bundle being parseable must inject a production issuer.
type NoopCertificateIssuer struct{}

// Issue returns a placeholder bundle.
func (NoopCertificateIssuer) Issue(_ context.Context, req CertificateRequest) (CertificateBundle, error) {
	now := time.Now()
	return CertificateBundle{
		CertPEM:   []byte("-----BEGIN CERTIFICATE-----\nnoop-" + req.CommonName + "\n-----END CERTIFICATE-----\n"),
		KeyPEM:    []byte("-----BEGIN PRIVATE KEY-----\nnoop\n-----END PRIVATE KEY-----\n"),
		CAPEM:     []byte("-----BEGIN CERTIFICATE-----\nnoop-ca\n-----END CERTIFICATE-----\n"),
		NotBefore: now,
		NotAfter:  now.Add(365 * 24 * time.Hour),
		Serial:    "noop-serial-" + req.Name,
	}, nil
}

// Revoke is a no-op.
func (NoopCertificateIssuer) Revoke(_ context.Context, _ string) error { return nil }
