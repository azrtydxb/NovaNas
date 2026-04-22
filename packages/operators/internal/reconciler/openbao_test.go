package reconciler

import (
	"context"
	"strings"
	"testing"
)

func TestRenderTenantPolicy_Default(t *testing.T) {
	hcl, err := RenderTenantPolicy("", "alice")
	if err != nil {
		t.Fatalf("RenderTenantPolicy: %v", err)
	}
	if !strings.Contains(hcl, `path "tenants/data/alice/*"`) {
		t.Errorf("expected tenant path interpolation, got:\n%s", hcl)
	}
	if !strings.Contains(hcl, `path "shared/data/tenants/alice"`) {
		t.Errorf("expected shared metadata path, got:\n%s", hcl)
	}
}

func TestRenderTenantPolicy_Custom(t *testing.T) {
	custom := `path "custom/{{.User}}" { capabilities = ["read"] }`
	hcl, err := RenderTenantPolicy(custom, "bob")
	if err != nil {
		t.Fatalf("RenderTenantPolicy: %v", err)
	}
	if !strings.Contains(hcl, `path "custom/bob"`) {
		t.Errorf("expected custom template interpolation, got:\n%s", hcl)
	}
}

func TestRenderTenantPolicy_EmptyUser(t *testing.T) {
	if _, err := RenderTenantPolicy("", ""); err == nil {
		t.Fatal("expected error on empty user")
	}
}

func TestFakeOpenBaoClient_PolicyLifecycle(t *testing.T) {
	ctx := context.Background()
	c := NewFakeOpenBaoClient()

	if err := c.EnsurePolicy(ctx, OpenBaoPolicy{Name: "tenant-alice", HCL: "path \"x\" { capabilities = [\"read\"] }"}); err != nil {
		t.Fatalf("EnsurePolicy: %v", err)
	}
	if _, ok := c.Policies["tenant-alice"]; !ok {
		t.Fatal("policy not recorded")
	}
	if err := c.DeletePolicy(ctx, "tenant-alice"); err != nil {
		t.Fatalf("DeletePolicy: %v", err)
	}
	if _, ok := c.Policies["tenant-alice"]; ok {
		t.Fatal("policy not removed")
	}
}

func TestFakeOpenBaoClient_AuthRoleLifecycle(t *testing.T) {
	ctx := context.Background()
	c := NewFakeOpenBaoClient()

	role := OpenBaoAuthRole{
		Name:                "tenant-alice",
		BoundServiceAccount: "alice",
		BoundNamespace:      "novanas-tenants",
		Policies:            []string{"tenant-alice"},
		TTLSeconds:          3600,
	}
	if err := c.EnsureAuthRole(ctx, role); err != nil {
		t.Fatalf("EnsureAuthRole: %v", err)
	}
	got, ok := c.Roles["tenant-alice"]
	if !ok {
		t.Fatal("role not recorded")
	}
	if got.BoundServiceAccount != "alice" {
		t.Errorf("bound SA mismatch: %s", got.BoundServiceAccount)
	}
	if err := c.DeleteAuthRole(ctx, "tenant-alice"); err != nil {
		t.Fatalf("DeleteAuthRole: %v", err)
	}
	if _, ok := c.Roles["tenant-alice"]; ok {
		t.Fatal("role not removed")
	}
}

func TestTenantNameHelpers(t *testing.T) {
	if got := TenantPolicyName("alice"); got != "tenant-alice" {
		t.Errorf("TenantPolicyName: got %q", got)
	}
	if got := TenantAuthRoleName("alice"); got != "tenant-alice" {
		t.Errorf("TenantAuthRoleName: got %q", got)
	}
}

func TestNoopOpenBaoClient(t *testing.T) {
	ctx := context.Background()
	var c OpenBaoClient = NoopOpenBaoClient{}
	if err := c.EnsurePolicy(ctx, OpenBaoPolicy{Name: "x", HCL: "y"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := c.DeletePolicy(ctx, "x"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := c.EnsureAuthRole(ctx, OpenBaoAuthRole{Name: "x"}); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
	if err := c.DeleteAuthRole(ctx, "x"); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}
