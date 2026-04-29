package plugins

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	hostls "github.com/novanas/nova-nas/internal/host/tls"
)

func ephemeralCA(t *testing.T) *hostls.Issuer {
	t.Helper()
	caCert, caKey, err := hostls.GenerateSelfSignedCA("test-ca", time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	cp := filepath.Join(dir, "ca.crt")
	kp := filepath.Join(dir, "ca.key")
	if err := os.WriteFile(cp, caCert, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kp, caKey, 0o600); err != nil {
		t.Fatal(err)
	}
	return &hostls.Issuer{CACertPath: cp, CAKeyPath: kp}
}

func TestTLSProvisioner_IssueAndShred(t *testing.T) {
	root := t.TempDir()
	p := &TLSCertProvisioner{Issuer: ephemeralCA(t), PluginsRoot: root}
	id, err := p.Provision(context.Background(), "rustfs", TLSCertNeed{
		CommonName: "rustfs.local",
		DNSNames:   []string{"rustfs"},
		IPs:        []string{"127.0.0.1"},
		TTLDays:    7,
	})
	if err != nil {
		t.Fatalf("provision: %v", err)
	}
	if id != "tlscert:rustfs/rustfs.local" {
		t.Errorf("id=%q", id)
	}
	certPath := filepath.Join(root, "rustfs", "certs", "cert.pem")
	keyPath := filepath.Join(root, "rustfs", "certs", "key.pem")
	if _, err := os.Stat(certPath); err != nil {
		t.Fatalf("cert missing: %v", err)
	}
	if _, err := os.Stat(keyPath); err != nil {
		t.Fatalf("key missing: %v", err)
	}

	// Idempotency: re-running does not error and does not regenerate.
	stat1, _ := os.Stat(certPath)
	if _, err := p.Provision(context.Background(), "rustfs", TLSCertNeed{CommonName: "rustfs.local"}); err != nil {
		t.Fatal(err)
	}
	stat2, _ := os.Stat(certPath)
	if !stat1.ModTime().Equal(stat2.ModTime()) {
		t.Error("cert was regenerated on idempotent re-run")
	}

	// Unprovision shreds files.
	if err := p.Unprovision(context.Background(), "rustfs", id); err != nil {
		t.Fatalf("unprovision: %v", err)
	}
	if _, err := os.Stat(certPath); !os.IsNotExist(err) {
		t.Error("cert not removed")
	}
	if _, err := os.Stat(keyPath); !os.IsNotExist(err) {
		t.Error("key not removed")
	}
}

func TestTLSProvisioner_RequiresCN(t *testing.T) {
	p := &TLSCertProvisioner{Issuer: ephemeralCA(t), PluginsRoot: t.TempDir()}
	if _, err := p.Provision(context.Background(), "p", TLSCertNeed{}); err == nil {
		t.Fatal("expected error")
	}
}
