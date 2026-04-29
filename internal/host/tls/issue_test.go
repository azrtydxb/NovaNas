package tls

import (
	"crypto/x509"
	"encoding/pem"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestIssue_RoundTripVerify(t *testing.T) {
	dir := t.TempDir()
	caCertPEM, caKeyPEM, err := GenerateSelfSignedCA("test-ca", 24*time.Hour)
	if err != nil {
		t.Fatalf("gen CA: %v", err)
	}
	caCertPath := filepath.Join(dir, "ca.crt")
	caKeyPath := filepath.Join(dir, "ca.key")
	if err := os.WriteFile(caCertPath, caCertPEM, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(caKeyPath, caKeyPEM, 0o600); err != nil {
		t.Fatal(err)
	}

	iss := &Issuer{CACertPath: caCertPath, CAKeyPath: caKeyPath}
	out, err := iss.Issue(LeafRequest{
		CommonName: "rustfs.local",
		DNSNames:   []string{"rustfs"},
		IPs:        []string{"127.0.0.1"},
		TTLDays:    7,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}

	// Parse the leaf cert and verify it chains to the CA.
	leafBlock, _ := pem.Decode(out.CertPEM)
	if leafBlock == nil {
		t.Fatal("no leaf PEM block")
	}
	leaf, err := x509.ParseCertificate(leafBlock.Bytes)
	if err != nil {
		t.Fatalf("parse leaf: %v", err)
	}
	caBlock, _ := pem.Decode(caCertPEM)
	caCert, err := x509.ParseCertificate(caBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}
	pool := x509.NewCertPool()
	pool.AddCert(caCert)
	if _, err := leaf.Verify(x509.VerifyOptions{
		Roots:     pool,
		KeyUsages: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		DNSName:   "rustfs.local",
	}); err != nil {
		t.Fatalf("verify: %v", err)
	}

	// Sanity: SANs propagated.
	if len(leaf.IPAddresses) != 1 || leaf.IPAddresses[0].String() != "127.0.0.1" {
		t.Errorf("ip SAN: %v", leaf.IPAddresses)
	}
	if !contains(leaf.DNSNames, "rustfs") || !contains(leaf.DNSNames, "rustfs.local") {
		t.Errorf("dns SANs: %v", leaf.DNSNames)
	}

	// And the key parses.
	keyBlock, _ := pem.Decode(out.KeyPEM)
	if keyBlock == nil {
		t.Fatal("no key PEM")
	}
	if _, err := x509.ParsePKCS1PrivateKey(keyBlock.Bytes); err != nil {
		t.Fatalf("parse key: %v", err)
	}
}

func TestIssue_MissingCA(t *testing.T) {
	iss := &Issuer{CACertPath: "/nonexistent/ca.crt", CAKeyPath: "/nonexistent/ca.key"}
	if _, err := iss.Issue(LeafRequest{CommonName: "x"}); err == nil {
		t.Fatal("expected error")
	}
}

func TestIssue_RequiresCommonName(t *testing.T) {
	iss := &Issuer{}
	if _, err := iss.Issue(LeafRequest{}); err == nil {
		t.Fatal("expected error")
	}
}

func contains(xs []string, want string) bool {
	for _, x := range xs {
		if x == want {
			return true
		}
	}
	return false
}
