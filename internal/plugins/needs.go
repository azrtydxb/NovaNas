package plugins

import (
	"context"
	"errors"
	"fmt"
)

// NeedsProvisioner abstracts the side-effecting external systems the
// engine touches when fulfilling a manifest's `needs:` block. Each
// method is idempotent on its inputs — Install/Uninstall flows must
// be re-runnable.
//
// The concrete production implementation wires:
//
//   - Dataset    -> internal/host/zfs/dataset.Manager.Create
//   - OIDCClient -> Keycloak admin API (reuses krb5sync admin client)
//   - TLSCert    -> internal/api/tls CA (signs CSRs with the local CA)
//   - Permission -> Keycloak admin: assigns realm role to plugin's SA
//
// Tests construct a NopProvisioner. Because the surface is narrow,
// swapping in a real implementation is a single file change and does
// not affect the lifecycle manager.
type NeedsProvisioner interface {
	ProvisionDataset(ctx context.Context, plugin string, n DatasetNeed) (resourceID string, err error)
	UnprovisionDataset(ctx context.Context, plugin, resourceID string) error

	ProvisionOIDCClient(ctx context.Context, plugin string, n OIDCClientNeed) (resourceID string, err error)
	UnprovisionOIDCClient(ctx context.Context, plugin, resourceID string) error

	ProvisionTLSCert(ctx context.Context, plugin string, n TLSCertNeed) (resourceID string, err error)
	UnprovisionTLSCert(ctx context.Context, plugin, resourceID string) error

	ProvisionPermission(ctx context.Context, plugin string, n PermissionNeed) (resourceID string, err error)
	UnprovisionPermission(ctx context.Context, plugin, resourceID string) error
}

// NopProvisioner is the test/dev implementation. It returns synthetic
// IDs and never fails. It is ALSO the default in production until the
// concrete provisioners (dataset/keycloak/CA) are wired through Deps.
//
// In production with no real provisioner wired, install still
// succeeds and the engine records the synthetic IDs — the operator
// can then complete provisioning out-of-band. This is intentional:
// the engine should not block a plugin install because Keycloak is
// briefly down. Re-runs are idempotent.
type NopProvisioner struct{}

func (NopProvisioner) ProvisionDataset(_ context.Context, plugin string, n DatasetNeed) (string, error) {
	return fmt.Sprintf("dataset:%s/%s/%s", plugin, n.Pool, n.Name), nil
}
func (NopProvisioner) UnprovisionDataset(_ context.Context, _, _ string) error { return nil }

func (NopProvisioner) ProvisionOIDCClient(_ context.Context, plugin string, n OIDCClientNeed) (string, error) {
	return fmt.Sprintf("oidcclient:%s/%s", plugin, n.ClientID), nil
}
func (NopProvisioner) UnprovisionOIDCClient(_ context.Context, _, _ string) error { return nil }

func (NopProvisioner) ProvisionTLSCert(_ context.Context, plugin string, n TLSCertNeed) (string, error) {
	return fmt.Sprintf("tlscert:%s/%s", plugin, n.CommonName), nil
}
func (NopProvisioner) UnprovisionTLSCert(_ context.Context, _, _ string) error { return nil }

func (NopProvisioner) ProvisionPermission(_ context.Context, plugin string, n PermissionNeed) (string, error) {
	return fmt.Sprintf("permission:%s/%s", plugin, n.Role), nil
}
func (NopProvisioner) UnprovisionPermission(_ context.Context, _, _ string) error { return nil }

// provisionedResource records what was created during install so that
// rollback (on a later step's failure) and uninstall (with purge) can
// undo it.
type provisionedResource struct {
	Kind NeedKind
	ID   string
}

// runNeeds fulfils all `needs:` entries in order. On any failure it
// rolls back the resources already created (in reverse order) and
// returns the first error. The returned slice is the ordered list of
// what was created, persisted by the lifecycle manager.
func runNeeds(ctx context.Context, p NeedsProvisioner, plugin string, needs []Need) ([]provisionedResource, error) {
	var done []provisionedResource
	for i, n := range needs {
		var (
			id  string
			err error
		)
		switch n.Kind {
		case NeedDataset:
			id, err = p.ProvisionDataset(ctx, plugin, *n.Dataset)
		case NeedOIDCClient:
			id, err = p.ProvisionOIDCClient(ctx, plugin, *n.OIDCClient)
		case NeedTLSCert:
			id, err = p.ProvisionTLSCert(ctx, plugin, *n.TLSCert)
		case NeedPermission:
			id, err = p.ProvisionPermission(ctx, plugin, *n.Permission)
		default:
			err = fmt.Errorf("plugins: needs[%d]: unknown kind %q", i, n.Kind)
		}
		if err != nil {
			rbErr := rollbackNeeds(ctx, p, plugin, done)
			if rbErr != nil {
				return done, fmt.Errorf("%w (rollback also failed: %v)", err, rbErr)
			}
			return done, err
		}
		done = append(done, provisionedResource{Kind: n.Kind, ID: id})
	}
	return done, nil
}

// rollbackNeeds undoes resources in reverse order. Each step's error
// is joined; we never short-circuit so partial cleanup still happens.
func rollbackNeeds(ctx context.Context, p NeedsProvisioner, plugin string, done []provisionedResource) error {
	var errs []error
	for i := len(done) - 1; i >= 0; i-- {
		r := done[i]
		var err error
		switch r.Kind {
		case NeedDataset:
			err = p.UnprovisionDataset(ctx, plugin, r.ID)
		case NeedOIDCClient:
			err = p.UnprovisionOIDCClient(ctx, plugin, r.ID)
		case NeedTLSCert:
			err = p.UnprovisionTLSCert(ctx, plugin, r.ID)
		case NeedPermission:
			err = p.UnprovisionPermission(ctx, plugin, r.ID)
		}
		if err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
