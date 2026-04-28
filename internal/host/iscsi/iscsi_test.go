package iscsi

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

// captureRunner records argv from each invocation so tests can assert
// on the exact targetcli call shape without executing anything.
type captureRunner struct {
	calls [][]string
}

func (c *captureRunner) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	cp := append([]string(nil), args...)
	c.calls = append(c.calls, cp)
	return nil, nil
}

func eq(a, b []string) bool { return reflect.DeepEqual(a, b) }

// ---------- backstore ----------

func TestBuildCreateBackstoreArgs_OK(t *testing.T) {
	got, err := buildCreateBackstoreArgs("zvol1", "/dev/zvol/tank/vol1")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/backstores/block", "create", "name=zvol1", "dev=/dev/zvol/tank/vol1"}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestBuildCreateBackstoreArgs_BadName(t *testing.T) {
	cases := []struct{ name, dev string }{
		{"", "/dev/zvol/tank/v"},
		{"-bad", "/dev/zvol/tank/v"},
		{"has space", "/dev/zvol/tank/v"},
		{"has/slash", "/dev/zvol/tank/v"},
	}
	for _, c := range cases {
		if _, err := buildCreateBackstoreArgs(c.name, c.dev); err == nil {
			t.Errorf("expected error for name=%q", c.name)
		}
	}
}

func TestBuildCreateBackstoreArgs_BadDev(t *testing.T) {
	cases := []string{
		"/etc/passwd",
		"-/dev/zvol/tank/v",
		"/dev/zvol/tank/v;rm",
		"/dev/zvol/tank/v with space",
	}
	for _, dev := range cases {
		if _, err := buildCreateBackstoreArgs("zvol1", dev); err == nil {
			t.Errorf("expected error for dev=%q", dev)
		}
	}
}

func TestManager_CreateBackstore(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{TargetcliBin: "/usr/bin/targetcli", Runner: cap.run}
	if err := m.CreateBackstore(context.Background(), "zvol1", "/dev/zvol/tank/vol1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"/backstores/block", "create", "name=zvol1", "dev=/dev/zvol/tank/vol1"}
	if len(cap.calls) != 1 || !eq(cap.calls[0], want) {
		t.Errorf("got=%v want=%v", cap.calls, want)
	}
}

func TestManager_DeleteBackstore(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{Runner: cap.run}
	if err := m.DeleteBackstore(context.Background(), "zvol1"); err != nil {
		t.Fatal(err)
	}
	want := []string{"/backstores/block", "delete", "name=zvol1"}
	if !eq(cap.calls[0], want) {
		t.Errorf("got=%v want=%v", cap.calls[0], want)
	}
}

// ---------- target ----------

func TestBuildCreateTargetArgs_OK(t *testing.T) {
	got, err := buildCreateTargetArgs("iqn.2020-01.io.example:tank")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/iscsi", "create", "wwn=iqn.2020-01.io.example:tank"}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestBuildCreateTargetArgs_BadIQN(t *testing.T) {
	cases := []string{
		"",
		"foo",                                   // missing iqn. prefix
		"-iqn.2020-01.io.example:tank",          // leading dash
		"iqn.2020-01.io.example:tank with space",
		"iqn.2020-01.io.example:tank;rm",
	}
	for _, iqn := range cases {
		if _, err := buildCreateTargetArgs(iqn); err == nil {
			t.Errorf("expected error for iqn=%q", iqn)
		}
	}
}

func TestManager_DeleteTarget(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{Runner: cap.run}
	if err := m.DeleteTarget(context.Background(), "iqn.2020-01.io.example:tank"); err != nil {
		t.Fatal(err)
	}
	want := []string{"/iscsi", "delete", "wwn=iqn.2020-01.io.example:tank"}
	if !eq(cap.calls[0], want) {
		t.Errorf("got=%v want=%v", cap.calls[0], want)
	}
}

func TestBuildListTargetsArgs(t *testing.T) {
	want := []string{"/iscsi", "ls", "depth=1"}
	if got := buildListTargetsArgs(); !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

// ---------- portal ----------

func TestBuildCreatePortalArgs_TCP(t *testing.T) {
	got, err := buildCreatePortalArgs("iqn.2020-01.io.example:tank",
		Portal{IP: "10.0.0.1", Port: 3260, Transport: "tcp"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/iscsi/iqn.2020-01.io.example:tank/tpg1/portals", "create",
		"ip_address=10.0.0.1", "ip_port=3260",
	}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestBuildEnableIserArgs(t *testing.T) {
	got, err := buildEnableIserArgs("iqn.2020-01.io.example:tank",
		Portal{IP: "10.0.0.1", Port: 3260, Transport: "iser"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/iscsi/iqn.2020-01.io.example:tank/tpg1/portals/10.0.0.1:3260",
		"enable_iser", "true",
	}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestManager_CreatePortal_ISER_RunsTwoCalls(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{Runner: cap.run}
	err := m.CreatePortal(context.Background(), "iqn.2020-01.io.example:tank",
		Portal{IP: "10.0.0.1", Port: 3260, Transport: "iser"})
	if err != nil {
		t.Fatal(err)
	}
	if len(cap.calls) != 2 {
		t.Fatalf("expected 2 calls, got %d: %v", len(cap.calls), cap.calls)
	}
	if cap.calls[1][1] != "enable_iser" {
		t.Errorf("second call should be enable_iser, got %v", cap.calls[1])
	}
}

func TestBuildCreatePortalArgs_BadIP(t *testing.T) {
	if _, err := buildCreatePortalArgs("iqn.2020-01.io.example:tank",
		Portal{IP: "not-an-ip", Port: 3260}); err == nil {
		t.Error("expected error for bad IP")
	}
}

func TestBuildCreatePortalArgs_BadPort(t *testing.T) {
	for _, port := range []int{0, -1, 70000} {
		if _, err := buildCreatePortalArgs("iqn.2020-01.io.example:tank",
			Portal{IP: "10.0.0.1", Port: port}); err == nil {
			t.Errorf("expected error for port=%d", port)
		}
	}
}

func TestBuildCreatePortalArgs_BadTransport(t *testing.T) {
	if _, err := buildCreatePortalArgs("iqn.2020-01.io.example:tank",
		Portal{IP: "10.0.0.1", Port: 3260, Transport: "rdma"}); err == nil {
		t.Error("expected error for bad transport")
	}
}

func TestBuildDeletePortalArgs(t *testing.T) {
	got, err := buildDeletePortalArgs("iqn.2020-01.io.example:tank",
		Portal{IP: "10.0.0.1", Port: 3260})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/iscsi/iqn.2020-01.io.example:tank/tpg1/portals", "delete",
		"ip_address=10.0.0.1", "ip_port=3260",
	}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

// ---------- LUN ----------

func TestBuildCreateLUNArgs_OK(t *testing.T) {
	got, err := buildCreateLUNArgs("iqn.2020-01.io.example:tank",
		LUN{ID: 1, Backstore: "zvol1", Zvol: "/dev/zvol/tank/vol1"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/iscsi/iqn.2020-01.io.example:tank/tpg1/luns", "create",
		"storage_object=/backstores/block/zvol1", "lun=1",
	}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestBuildCreateLUNArgs_NegativeID(t *testing.T) {
	if _, err := buildCreateLUNArgs("iqn.2020-01.io.example:tank",
		LUN{ID: -1, Backstore: "zvol1"}); err == nil {
		t.Error("expected error for negative LUN id")
	}
}

func TestBuildCreateLUNArgs_BadBackstore(t *testing.T) {
	if _, err := buildCreateLUNArgs("iqn.2020-01.io.example:tank",
		LUN{ID: 0, Backstore: "-evil"}); err == nil {
		t.Error("expected error for backstore with leading dash")
	}
}

func TestBuildDeleteLUNArgs(t *testing.T) {
	got, err := buildDeleteLUNArgs("iqn.2020-01.io.example:tank", 2)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"/iscsi/iqn.2020-01.io.example:tank/tpg1/luns", "delete", "2"}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

// ---------- ACL ----------

func TestBuildCreateACLArgs_NoCHAP(t *testing.T) {
	got, err := buildCreateACLArgs("iqn.2020-01.io.example:tank",
		ACL{InitiatorIQN: "iqn.2020-01.io.client:host"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/iscsi/iqn.2020-01.io.example:tank/tpg1/acls", "create",
		"wwn=iqn.2020-01.io.client:host",
	}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestBuildCreateACLArgs_BadInitiator(t *testing.T) {
	if _, err := buildCreateACLArgs("iqn.2020-01.io.example:tank",
		ACL{InitiatorIQN: "-iqn.malicious"}); err == nil {
		t.Error("expected error for initiator with leading dash")
	}
}

func TestBuildSetCHAPUserArgs_OK(t *testing.T) {
	got, err := buildSetCHAPUserArgs("iqn.2020-01.io.example:tank",
		ACL{InitiatorIQN: "iqn.2020-01.io.client:host", CHAPUser: "alice"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/iscsi/iqn.2020-01.io.example:tank/tpg1/acls/iqn.2020-01.io.client:host",
		"set", "auth", "userid=alice",
	}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestBuildSetCHAPPasswordArgs_OK(t *testing.T) {
	got, err := buildSetCHAPPasswordArgs("iqn.2020-01.io.example:tank",
		ACL{InitiatorIQN: "iqn.2020-01.io.client:host", CHAPSecret: "supersecret123"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/iscsi/iqn.2020-01.io.example:tank/tpg1/acls/iqn.2020-01.io.client:host",
		"set", "auth", "password=supersecret123",
	}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

func TestBuildSetCHAPPasswordArgs_BadLength(t *testing.T) {
	for _, secret := range []string{"short", "thispasswordistoolong"} {
		_, err := buildSetCHAPPasswordArgs("iqn.2020-01.io.example:tank",
			ACL{InitiatorIQN: "iqn.2020-01.io.client:host", CHAPSecret: secret})
		if err == nil {
			t.Errorf("expected length error for secret=%q", secret)
		}
	}
}

func TestBuildSetCHAPPasswordArgs_ShellMeta(t *testing.T) {
	_, err := buildSetCHAPPasswordArgs("iqn.2020-01.io.example:tank",
		ACL{InitiatorIQN: "iqn.2020-01.io.client:host", CHAPSecret: "abc;rm -rf"})
	if err == nil {
		t.Error("expected error for shell metacharacter in secret")
	}
}

func TestBuildSetCHAPUserArgs_ControlChar(t *testing.T) {
	_, err := buildSetCHAPUserArgs("iqn.2020-01.io.example:tank",
		ACL{InitiatorIQN: "iqn.2020-01.io.client:host", CHAPUser: "alice\x00"})
	if err == nil {
		t.Error("expected error for control char in CHAP user")
	}
}

func TestManager_CreateACL_WithCHAP_RunsThreeCalls(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{Runner: cap.run}
	err := m.CreateACL(context.Background(), "iqn.2020-01.io.example:tank", ACL{
		InitiatorIQN: "iqn.2020-01.io.client:host",
		CHAPUser:     "alice",
		CHAPSecret:   "supersecret123",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(cap.calls) != 3 {
		t.Fatalf("expected 3 calls, got %d: %v", len(cap.calls), cap.calls)
	}
	// Order: create acl, set userid, set password.
	if cap.calls[0][1] != "create" {
		t.Errorf("call[0] should be create: %v", cap.calls[0])
	}
	if cap.calls[1][2] != "auth" || !strings.HasPrefix(cap.calls[1][3], "userid=") {
		t.Errorf("call[1] should set userid: %v", cap.calls[1])
	}
	if cap.calls[2][2] != "auth" || !strings.HasPrefix(cap.calls[2][3], "password=") {
		t.Errorf("call[2] should set password: %v", cap.calls[2])
	}
}

func TestManager_CreateACL_BadCHAP_NoCalls(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{Runner: cap.run}
	err := m.CreateACL(context.Background(), "iqn.2020-01.io.example:tank", ACL{
		InitiatorIQN: "iqn.2020-01.io.client:host",
		CHAPUser:     "alice",
		CHAPSecret:   "tooshort", // < 12 chars
	})
	if err == nil {
		t.Fatal("expected error for short CHAP secret")
	}
	if len(cap.calls) != 0 {
		t.Errorf("expected 0 calls (validation must reject before run), got %v", cap.calls)
	}
}

func TestBuildDeleteACLArgs(t *testing.T) {
	got, err := buildDeleteACLArgs("iqn.2020-01.io.example:tank", "iqn.2020-01.io.client:host")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"/iscsi/iqn.2020-01.io.example:tank/tpg1/acls", "delete",
		"wwn=iqn.2020-01.io.client:host",
	}
	if !eq(got, want) {
		t.Errorf("got=%v want=%v", got, want)
	}
}

// ---------- saveconfig ----------

func TestManager_SaveConfig(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{Runner: cap.run}
	if err := m.SaveConfig(context.Background()); err != nil {
		t.Fatal(err)
	}
	want := []string{"saveconfig"}
	if !eq(cap.calls[0], want) {
		t.Errorf("got=%v want=%v", cap.calls[0], want)
	}
}

// ---------- bin default ----------

func TestManager_BinDefault(t *testing.T) {
	cap := &captureRunner{}
	m := &Manager{Runner: func(ctx context.Context, bin string, args ...string) ([]byte, error) {
		if bin != "/usr/bin/targetcli" {
			t.Errorf("expected default bin /usr/bin/targetcli, got %q", bin)
		}
		return cap.run(ctx, bin, args...)
	}}
	_ = m.SaveConfig(context.Background())
}
