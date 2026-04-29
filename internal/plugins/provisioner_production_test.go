package plugins

import (
	"context"
	"testing"
)

// TestProductionProvisioner_FullCycle drives all four provisioners
// through a typical install order (dataset → oidc → tlsCert →
// permission) and a uninstall (reverse order), against the in-memory
// fakes used by the per-provisioner tests.
func TestProductionProvisioner_FullCycle(t *testing.T) {
	fk := newFakeKeycloak(t)
	fk.realmRoles["rustfs-admin"] = &kcRole{ID: "role-rustfs-admin", Name: "rustfs-admin"}
	dsClient := newFakeDatasetClient()
	root := t.TempDir()
	pp := NewProductionProvisioner(
		&DatasetProvisioner{Client: dsClient},
		&OIDCClientProvisioner{Admin: fk.doer(), Secrets: newMemSecrets()},
		&TLSCertProvisioner{Issuer: ephemeralCA(t), PluginsRoot: root},
		&PermissionProvisioner{Admin: fk.doer()},
	)

	needs := []Need{
		{Kind: NeedDataset, Dataset: &DatasetNeed{Pool: "tank", Name: "rustfs/data"}},
		{Kind: NeedOIDCClient, OIDCClient: &OIDCClientNeed{ClientID: "rustfs"}},
		{Kind: NeedTLSCert, TLSCert: &TLSCertNeed{CommonName: "rustfs.local"}},
		{Kind: NeedPermission, Permission: &PermissionNeed{Role: "rustfs-admin"}},
	}
	got, err := runNeeds(context.Background(), pp, "rustfs", needs)
	if err != nil {
		t.Fatalf("runNeeds: %v", err)
	}
	if len(got) != 4 {
		t.Fatalf("provisioned=%d", len(got))
	}

	if !fk.users["sa-user-rustfs"]["rustfs-admin"] {
		t.Error("permission not bound to SA user")
	}

	// Idempotent re-run — same set of needs returns cleanly.
	if _, err := runNeeds(context.Background(), pp, "rustfs", needs); err != nil {
		t.Fatalf("re-runNeeds: %v", err)
	}

	if err := rollbackNeeds(context.Background(), pp, "rustfs", got); err != nil {
		t.Fatalf("rollback: %v", err)
	}
	if fk.users["sa-user-rustfs"]["rustfs-admin"] {
		t.Error("permission still bound after rollback")
	}
	if fk.deleteCalls != 1 {
		t.Errorf("expected 1 client delete, got %d", fk.deleteCalls)
	}
	if len(dsClient.destroyed) != 1 {
		t.Errorf("expected 1 dataset destroy, got %v", dsClient.destroyed)
	}
}

func TestProductionProvisioner_FailoverInUnconfigured(t *testing.T) {
	pp := NewProductionProvisioner(nil, nil, nil, nil)
	if _, err := pp.ProvisionDataset(context.Background(), "p", DatasetNeed{Pool: "t", Name: "n"}); err == nil {
		t.Error("expected error for unconfigured dataset provisioner")
	}
	if _, err := pp.ProvisionOIDCClient(context.Background(), "p", OIDCClientNeed{ClientID: "c"}); err == nil {
		t.Error("expected error for unconfigured oidc provisioner")
	}
	if _, err := pp.ProvisionTLSCert(context.Background(), "p", TLSCertNeed{CommonName: "x"}); err == nil {
		t.Error("expected error for unconfigured tls provisioner")
	}
	if _, err := pp.ProvisionPermission(context.Background(), "p", PermissionNeed{Role: "r"}); err == nil {
		t.Error("expected error for unconfigured permission provisioner")
	}
}
