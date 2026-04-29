package plugins

import (
	"context"
	"fmt"
	"sync"
)

// ProductionProvisioner is the composed NeedsProvisioner used by the
// running nova-api binary. It dispatches each NeedKind to its
// dedicated provisioner and shares context (e.g. the OIDC clientId)
// across needs that depend on each other.
//
// Construction is "wire what you have" — any sub-provisioner left nil
// causes its Need kind to fail with a clear error. The composed type
// is safe for concurrent installs but each plugin install is already
// serialized by the lifecycle manager's per-name mutex.
type ProductionProvisioner struct {
	Dataset    *DatasetProvisioner
	OIDC       *OIDCClientProvisioner
	TLS        *TLSCertProvisioner
	Permission *PermissionProvisioner

	mu              sync.Mutex
	pluginClientIDs map[string]string // plugin -> last-provisioned OIDC clientId
}

// NewProductionProvisioner builds a ProductionProvisioner and wires
// the Permission sub-provisioner's ClientIDFor callback to look at
// the in-memory map populated during ProvisionOIDCClient. This makes
// PermissionNeed work for plugins whose oidcClient.clientId differs
// from the plugin name.
func NewProductionProvisioner(ds *DatasetProvisioner, oidc *OIDCClientProvisioner, tls *TLSCertProvisioner, perm *PermissionProvisioner) *ProductionProvisioner {
	pp := &ProductionProvisioner{
		Dataset:         ds,
		OIDC:            oidc,
		TLS:             tls,
		Permission:      perm,
		pluginClientIDs: map[string]string{},
	}
	if perm != nil && perm.ClientIDFor == nil {
		perm.ClientIDFor = pp.lookupClientID
	}
	return pp
}

func (p *ProductionProvisioner) rememberClientID(plugin, clientID string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.pluginClientIDs[plugin] = clientID
}

func (p *ProductionProvisioner) lookupClientID(plugin string) string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.pluginClientIDs[plugin]
}

// ProvisionDataset implements NeedsProvisioner.
func (p *ProductionProvisioner) ProvisionDataset(ctx context.Context, plugin string, n DatasetNeed) (string, error) {
	if p.Dataset == nil {
		return "", fmt.Errorf("plugins: dataset provisioner not configured")
	}
	return p.Dataset.Provision(ctx, plugin, n)
}

// UnprovisionDataset implements NeedsProvisioner.
func (p *ProductionProvisioner) UnprovisionDataset(ctx context.Context, plugin, id string) error {
	if p.Dataset == nil {
		return fmt.Errorf("plugins: dataset provisioner not configured")
	}
	return p.Dataset.Unprovision(ctx, plugin, id)
}

// ProvisionOIDCClient implements NeedsProvisioner.
func (p *ProductionProvisioner) ProvisionOIDCClient(ctx context.Context, plugin string, n OIDCClientNeed) (string, error) {
	if p.OIDC == nil {
		return "", fmt.Errorf("plugins: oidc provisioner not configured")
	}
	id, err := p.OIDC.Provision(ctx, plugin, n)
	if err == nil {
		p.rememberClientID(plugin, n.ClientID)
	}
	return id, err
}

// UnprovisionOIDCClient implements NeedsProvisioner.
func (p *ProductionProvisioner) UnprovisionOIDCClient(ctx context.Context, plugin, id string) error {
	if p.OIDC == nil {
		return fmt.Errorf("plugins: oidc provisioner not configured")
	}
	return p.OIDC.Unprovision(ctx, plugin, id)
}

// ProvisionTLSCert implements NeedsProvisioner.
func (p *ProductionProvisioner) ProvisionTLSCert(ctx context.Context, plugin string, n TLSCertNeed) (string, error) {
	if p.TLS == nil {
		return "", fmt.Errorf("plugins: tls provisioner not configured")
	}
	return p.TLS.Provision(ctx, plugin, n)
}

// UnprovisionTLSCert implements NeedsProvisioner.
func (p *ProductionProvisioner) UnprovisionTLSCert(ctx context.Context, plugin, id string) error {
	if p.TLS == nil {
		return fmt.Errorf("plugins: tls provisioner not configured")
	}
	return p.TLS.Unprovision(ctx, plugin, id)
}

// ProvisionPermission implements NeedsProvisioner.
func (p *ProductionProvisioner) ProvisionPermission(ctx context.Context, plugin string, n PermissionNeed) (string, error) {
	if p.Permission == nil {
		return "", fmt.Errorf("plugins: permission provisioner not configured")
	}
	return p.Permission.Provision(ctx, plugin, n)
}

// UnprovisionPermission implements NeedsProvisioner.
func (p *ProductionProvisioner) UnprovisionPermission(ctx context.Context, plugin, id string) error {
	if p.Permission == nil {
		return fmt.Errorf("plugins: permission provisioner not configured")
	}
	return p.Permission.Unprovision(ctx, plugin, id)
}
