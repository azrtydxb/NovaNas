package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	hostls "github.com/novanas/nova-nas/internal/host/tls"
)

// TLSCertProvisioner mints a CA-signed server cert and writes it under
// <PluginsRoot>/<plugin>/certs/{cert.pem,key.pem}.
type TLSCertProvisioner struct {
	Issuer      *hostls.Issuer
	PluginsRoot string
	Logger      *slog.Logger
}

func tlsResourceID(plugin, cn string) string {
	return fmt.Sprintf("tlscert:%s/%s", plugin, cn)
}

func parseTLSResourceID(id string) (plugin, cn string, ok bool) {
	if !strings.HasPrefix(id, "tlscert:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(id, "tlscert:")
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", "", false
	}
	return rest[:slash], rest[slash+1:], true
}

func (p *TLSCertProvisioner) certDir(plugin string) string {
	root := p.PluginsRoot
	if root == "" {
		root = DefaultPluginsRoot
	}
	return filepath.Join(root, plugin, "certs")
}

// Provision issues + writes the cert. Idempotent: if both files exist
// already, the cert is left alone.
func (p *TLSCertProvisioner) Provision(ctx context.Context, plugin string, n TLSCertNeed) (string, error) {
	if p.Issuer == nil {
		return "", errors.New("plugins: TLSCertProvisioner.Issuer is nil")
	}
	if n.CommonName == "" {
		return "", fmt.Errorf("plugins: tlsCert need: commonName required")
	}
	dir := p.certDir(plugin)
	certPath := filepath.Join(dir, "cert.pem")
	keyPath := filepath.Join(dir, "key.pem")
	if _, err := os.Stat(certPath); err == nil {
		if _, err := os.Stat(keyPath); err == nil {
			if p.Logger != nil {
				p.Logger.Info("plugins: tlsCert already exists; reusing", "plugin", plugin, "cn", n.CommonName)
			}
			return tlsResourceID(plugin, n.CommonName), nil
		}
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", fmt.Errorf("plugins: mkdir cert dir: %w", err)
	}
	out, err := p.Issuer.Issue(hostls.LeafRequest{
		CommonName: n.CommonName,
		DNSNames:   n.DNSNames,
		IPs:        n.IPs,
		TTLDays:    n.TTLDays,
	})
	if err != nil {
		return "", fmt.Errorf("plugins: tlsCert issue: %w", err)
	}
	if err := os.WriteFile(certPath, out.CertPEM, 0o644); err != nil {
		return "", fmt.Errorf("plugins: write cert: %w", err)
	}
	if err := os.WriteFile(keyPath, out.KeyPEM, 0o640); err != nil {
		return "", fmt.Errorf("plugins: write key: %w", err)
	}
	if p.Logger != nil {
		p.Logger.Info("plugins: tlsCert issued", "plugin", plugin, "cn", n.CommonName, "not_after", out.NotAfter)
	}
	_ = ctx
	return tlsResourceID(plugin, n.CommonName), nil
}

// Unprovision shreds the cert + key files.
func (p *TLSCertProvisioner) Unprovision(_ context.Context, plugin, resourceID string) error {
	rPlugin, _, ok := parseTLSResourceID(resourceID)
	if !ok || rPlugin != plugin {
		return fmt.Errorf("plugins: bad tlsCert resource id %q", resourceID)
	}
	dir := p.certDir(plugin)
	for _, f := range []string{"cert.pem", "key.pem"} {
		path := filepath.Join(dir, f)
		// Best-effort overwrite-then-remove. Failure to overwrite is
		// non-fatal (file may already be gone).
		if data, err := os.ReadFile(path); err == nil {
			zeros := make([]byte, len(data))
			_ = os.WriteFile(path, zeros, 0o600)
		}
		_ = os.Remove(path)
	}
	if p.Logger != nil {
		p.Logger.Info("plugins: tlsCert removed", "plugin", plugin, "id", resourceID)
	}
	return nil
}
