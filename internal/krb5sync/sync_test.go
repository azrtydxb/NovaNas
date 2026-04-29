package krb5sync

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"sync"
	"testing"
	"time"

	novanas "github.com/novanas/nova-nas/clients/go/novanas"
)

// fakeKeycloak satisfies KeycloakAPI in-memory.
type fakeKeycloak struct {
	users    []KeycloakUser
	events   []AdminEvent
	listErr  error
	getErr   error
	eventErr error
}

func (f *fakeKeycloak) ListUsers(ctx context.Context) ([]KeycloakUser, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.users, nil
}
func (f *fakeKeycloak) GetUser(ctx context.Context, id string) (*KeycloakUser, error) {
	if f.getErr != nil {
		return nil, f.getErr
	}
	for i := range f.users {
		if f.users[i].ID == id {
			u := f.users[i]
			return &u, nil
		}
	}
	return nil, nil
}
func (f *fakeKeycloak) ListAdminEvents(ctx context.Context, since time.Time) ([]AdminEvent, error) {
	if f.eventErr != nil {
		return nil, f.eventErr
	}
	return f.events, nil
}

// fakeKDC satisfies PrincipalAPI in-memory.
type fakeKDC struct {
	mu        sync.Mutex
	exists    map[string]struct{}
	createErr map[string]error
	deleteErr map[string]error
	creates   []string
	deletes   []string
}

func newFakeKDC(initial ...string) *fakeKDC {
	f := &fakeKDC{exists: map[string]struct{}{}}
	for _, n := range initial {
		f.exists[n] = struct{}{}
	}
	return f
}

func (f *fakeKDC) ListPrincipals(ctx context.Context) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.exists))
	for n := range f.exists {
		out = append(out, n)
	}
	sort.Strings(out)
	return out, nil
}

func (f *fakeKDC) CreatePrincipal(ctx context.Context, spec novanas.CreatePrincipalSpec) (*novanas.Principal, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.createErr[spec.Name]; ok {
		return nil, err
	}
	f.exists[spec.Name] = struct{}{}
	f.creates = append(f.creates, spec.Name)
	return &novanas.Principal{Name: spec.Name}, nil
}

func (f *fakeKDC) DeletePrincipal(ctx context.Context, name string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if err, ok := f.deleteErr[name]; ok {
		return err
	}
	delete(f.exists, name)
	f.deletes = append(f.deletes, name)
	return nil
}

func TestExpectedPrincipals(t *testing.T) {
	cases := []struct {
		name string
		u    KeycloakUser
		want []string
	}{
		{
			"disabled user gets nothing",
			KeycloakUser{Username: "alice", Enabled: false, Attributes: map[string][]string{TenantAttribute: {"acme"}}},
			nil,
		},
		{
			"single tenant",
			KeycloakUser{Username: "alice", Enabled: true, Attributes: map[string][]string{TenantAttribute: {"acme"}}},
			[]string{"alice/acme@NOVANAS.LOCAL"},
		},
		{
			"multi-tenant",
			KeycloakUser{Username: "alice", Enabled: true, Attributes: map[string][]string{TenantAttribute: {"acme", "foo"}}},
			[]string{"alice/acme@NOVANAS.LOCAL", "alice/foo@NOVANAS.LOCAL"},
		},
		{
			"no tenant + no platform-nfs gets nothing",
			KeycloakUser{Username: "ops", Enabled: true},
			nil,
		},
		{
			"no tenant + platform-nfs gets bare principal",
			KeycloakUser{Username: "ops", Enabled: true, Attributes: map[string][]string{PlatformNFSAttribute: {"true"}}},
			[]string{"ops@NOVANAS.LOCAL"},
		},
		{
			"empty username",
			KeycloakUser{Enabled: true, Attributes: map[string][]string{TenantAttribute: {"acme"}}},
			nil,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := ExpectedPrincipals(tc.u, "NOVANAS.LOCAL")
			if !reflect.DeepEqual(got, tc.want) {
				t.Errorf("got %v, want %v", got, tc.want)
			}
		})
	}
}

func TestIsServicePrincipal(t *testing.T) {
	for _, name := range []string{
		"krbtgt/NOVANAS.LOCAL@NOVANAS.LOCAL",
		"kadmin/admin@NOVANAS.LOCAL",
		"nfs/host.example@NOVANAS.LOCAL",
		"host/example@NOVANAS.LOCAL",
		"K/M",
	} {
		if !IsServicePrincipal(name) {
			t.Errorf("expected %q to be a service principal", name)
		}
	}
	for _, name := range []string{
		"alice@NOVANAS.LOCAL",
		"alice/acme@NOVANAS.LOCAL",
	} {
		if IsServicePrincipal(name) {
			t.Errorf("expected %q NOT to be a service principal", name)
		}
	}
}

func TestPrincipalUserPattern(t *testing.T) {
	realm := "NOVANAS.LOCAL"
	cases := map[string]bool{
		"alice@NOVANAS.LOCAL":           true,
		"alice/acme@NOVANAS.LOCAL":      true,
		"nfs/host@NOVANAS.LOCAL":        false, // service
		"krbtgt/x@NOVANAS.LOCAL":        false,
		"alice@OTHER.REALM":             false,
		"":                              false,
		"@NOVANAS.LOCAL":                false,
		"a/b/c@NOVANAS.LOCAL":           false, // 2 slashes
	}
	for in, want := range cases {
		if got := PrincipalUserPattern(in, realm); got != want {
			t.Errorf("PrincipalUserPattern(%q)=%v want %v", in, got, want)
		}
	}
}

func TestReconcileOnceCreatesAndDeletes(t *testing.T) {
	kc := &fakeKeycloak{users: []KeycloakUser{
		{ID: "u1", Username: "alice", Enabled: true, Attributes: map[string][]string{TenantAttribute: {"acme"}}},
		{ID: "u2", Username: "bob", Enabled: true, Attributes: map[string][]string{TenantAttribute: {"acme", "foo"}}},
	}}
	// KDC starts with: a service principal we must never touch, an old
	// principal whose user is gone (must be deleted), and one we keep.
	kdc := newFakeKDC(
		"nfs/host@NOVANAS.LOCAL",         // service, never touched
		"krbtgt/NOVANAS.LOCAL@NOVANAS.LOCAL",
		"alice/acme@NOVANAS.LOCAL",       // already correct
		"orphan/old@NOVANAS.LOCAL",       // user-shaped but no Keycloak match -> delete
	)
	st := NewMemState(NewState())
	rec := NewReconciler(kc, kdc, st, Config{Realm: "NOVANAS.LOCAL"})

	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("ReconcileOnce: %v", err)
	}

	wantCreated := []string{"bob/acme@NOVANAS.LOCAL", "bob/foo@NOVANAS.LOCAL"}
	sort.Strings(res.Created)
	if !reflect.DeepEqual(res.Created, wantCreated) {
		t.Errorf("created=%v, want %v", res.Created, wantCreated)
	}
	wantDeleted := []string{"orphan/old@NOVANAS.LOCAL"}
	if !reflect.DeepEqual(res.Deleted, wantDeleted) {
		t.Errorf("deleted=%v, want %v", res.Deleted, wantDeleted)
	}

	// Service principals must remain untouched.
	if _, ok := kdc.exists["nfs/host@NOVANAS.LOCAL"]; !ok {
		t.Errorf("service principal nfs/host was deleted")
	}
	if _, ok := kdc.exists["krbtgt/NOVANAS.LOCAL@NOVANAS.LOCAL"]; !ok {
		t.Errorf("service principal krbtgt was deleted")
	}

	// State updated.
	snap := st.Snapshot()
	if got := snap.UserPrincipals["u1"]; !reflect.DeepEqual(got, []string{"alice/acme@NOVANAS.LOCAL"}) {
		t.Errorf("state[u1]=%v", got)
	}
	if got := snap.UserPrincipals["u2"]; !reflect.DeepEqual(got, []string{"bob/acme@NOVANAS.LOCAL", "bob/foo@NOVANAS.LOCAL"}) {
		t.Errorf("state[u2]=%v", got)
	}
}

func TestReconcileFailSoftPerPrincipal(t *testing.T) {
	kc := &fakeKeycloak{users: []KeycloakUser{
		{ID: "u1", Username: "alice", Enabled: true, Attributes: map[string][]string{TenantAttribute: {"acme"}}},
		{ID: "u2", Username: "bob", Enabled: true, Attributes: map[string][]string{TenantAttribute: {"acme"}}},
	}}
	kdc := newFakeKDC()
	kdc.createErr = map[string]error{
		"alice/acme@NOVANAS.LOCAL": errors.New("simulated failure"),
	}
	rec := NewReconciler(kc, kdc, NewMemState(NewState()), Config{Realm: "NOVANAS.LOCAL"})

	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("ReconcileOnce: %v", err)
	}
	if res.CreateErrors != 1 {
		t.Errorf("CreateErrors=%d, want 1", res.CreateErrors)
	}
	// bob should still have been created despite alice's failure.
	if _, ok := kdc.exists["bob/acme@NOVANAS.LOCAL"]; !ok {
		t.Errorf("bob should have been created despite alice failure")
	}
}

func TestReconcileKeycloakErrorIsHardFail(t *testing.T) {
	kc := &fakeKeycloak{listErr: errors.New("kc unreachable")}
	kdc := newFakeKDC()
	rec := NewReconciler(kc, kdc, NewMemState(NewState()), Config{Realm: "NOVANAS.LOCAL"})
	if _, err := rec.ReconcileOnce(context.Background()); err == nil {
		t.Errorf("expected error when keycloak unreachable")
	}
}

func TestReconcileDisabledUserPrincipalIsDeleted(t *testing.T) {
	// alice was enabled previously, now disabled — her principal should
	// be deleted on next reconcile.
	kc := &fakeKeycloak{users: []KeycloakUser{
		{ID: "u1", Username: "alice", Enabled: false, Attributes: map[string][]string{TenantAttribute: {"acme"}}},
	}}
	kdc := newFakeKDC("alice/acme@NOVANAS.LOCAL")
	rec := NewReconciler(kc, kdc, NewMemState(NewState()), Config{Realm: "NOVANAS.LOCAL"})
	res, err := rec.ReconcileOnce(context.Background())
	if err != nil {
		t.Fatalf("ReconcileOnce: %v", err)
	}
	if got := res.Deleted; !reflect.DeepEqual(got, []string{"alice/acme@NOVANAS.LOCAL"}) {
		t.Errorf("deleted=%v, want [alice/acme@NOVANAS.LOCAL]", got)
	}
}

func TestProcessAdminEventsSignalsChange(t *testing.T) {
	kc := &fakeKeycloak{events: []AdminEvent{
		{Time: 1700000000000, OperationType: "CREATE", ResourceType: "USER", ResourcePath: "users/u1"},
		{Time: 1700000050000, OperationType: "UPDATE", ResourceType: "OTHER", ResourcePath: "groups/x"},
	}}
	kdc := newFakeKDC()
	st := NewMemState(NewState())
	rec := NewReconciler(kc, kdc, st, Config{Realm: "NOVANAS.LOCAL"})
	changed, err := rec.processAdminEvents(context.Background())
	if err != nil {
		t.Fatalf("processAdminEvents: %v", err)
	}
	if !changed {
		t.Errorf("expected changed=true")
	}
	if st.LastEvent().IsZero() {
		t.Errorf("expected LastEvent stamped")
	}
}
