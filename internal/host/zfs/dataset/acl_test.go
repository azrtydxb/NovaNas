package dataset

import (
	"context"
	"errors"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/novanas/nova-nas/internal/host/exec"
)

func TestValidatePath(t *testing.T) {
	good := []string{
		"/tank/home",
		"/tank/home/alice",
		"/a",
	}
	for _, p := range good {
		if err := validatePath(p); err != nil {
			t.Errorf("validatePath(%q): %v", p, err)
		}
	}
	bad := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"relative", "tank/home"},
		{"leading-dash", "-rf"},
		{"traversal", "/tank/../etc"},
		{"semicolon", "/tank;rm"},
		{"newline", "/tank\nfoo"},
		{"backtick", "/tank`id`"},
		{"dollar", "/tank/$x"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if err := validatePath(c.in); err == nil {
				t.Errorf("expected error for %q", c.in)
			}
		})
	}
}

func TestValidatePrincipal(t *testing.T) {
	good := []string{
		"user:alice",
		"user:alice.smith",
		"user:DOMAIN\\alice",
		"user:alice@example.com",
		"group:eng",
		"OWNER@",
		"GROUP@",
		"EVERYONE@",
	}
	for _, p := range good {
		t.Run(p, func(t *testing.T) {
			if err := validatePrincipal(p); err != nil {
				t.Errorf("validatePrincipal(%q): %v", p, err)
			}
		})
	}
	bad := []struct {
		name string
		in   string
	}{
		{"empty", ""},
		{"no-prefix", "alice"},
		{"empty-name", "user:"},
		{"lowercase-special", "owner@"},
		{"mixedcase-special", "Owner@"},
		{"space-in-name", "user:alice smith"},
		{"semicolon", "user:alice;rm"},
		{"unknown-prefix", "role:admin"},
	}
	for _, c := range bad {
		t.Run(c.name, func(t *testing.T) {
			if err := validatePrincipal(c.in); err == nil {
				t.Errorf("expected error for %q", c.in)
			}
		})
	}
}

func TestACEToString(t *testing.T) {
	cases := []struct {
		name string
		ace  ACE
		want string
	}{
		{
			"allow-owner-read",
			ACE{Type: ACETypeAllow, Principal: "OWNER@", Permissions: []ACLPerm{PermRead}},
			"A::OWNER@:r",
		},
		{
			"deny-everyone-write",
			ACE{Type: ACETypeDeny, Principal: "EVERYONE@", Permissions: []ACLPerm{PermWrite}},
			"D::EVERYONE@:w",
		},
		{
			"group-auto-g-flag",
			ACE{Type: ACETypeAllow, Principal: "GROUP@", Permissions: []ACLPerm{PermRead, PermExecute}},
			"A:g:GROUP@:rx",
		},
		{
			"user-bare",
			ACE{Type: ACETypeAllow, Principal: "user:alice", Permissions: []ACLPerm{PermRead, PermWrite}},
			"A::alice:rw",
		},
		{
			"group-with-trailing-at",
			ACE{Type: ACETypeAllow, Principal: "group:eng", Permissions: []ACLPerm{PermRead}},
			"A:g:eng@:r",
		},
		{
			"group-with-domain-no-extra-at",
			ACE{Type: ACETypeAllow, Principal: "group:eng@DOMAIN", Permissions: []ACLPerm{PermRead}},
			"A:g:eng@DOMAIN:r",
		},
		{
			"inheritance-file-and-dir",
			ACE{
				Type:        ACETypeAllow,
				Principal:   "user:alice",
				Permissions: []ACLPerm{PermRead},
				Inheritance: []InheritFlag{InheritFile, InheritDir},
			},
			"A:fd:alice:r",
		},
		{
			"perm-canonical-order",
			// Inputs deliberately scrambled.
			ACE{Type: ACETypeAllow, Principal: "user:alice",
				Permissions: []ACLPerm{PermSync, PermRead, PermWriteOwner, PermWrite}},
			"A::alice:rwoy",
		},
		{
			"shorthand-full-control",
			ACE{Type: ACETypeAllow, Principal: "OWNER@", Permissions: []ACLPerm{PermFullControl}},
			"A::OWNER@:rwxaDdtTnNcCoy",
		},
		{
			"shorthand-modify",
			ACE{Type: ACETypeAllow, Principal: "user:alice", Permissions: []ACLPerm{PermModify}},
			"A::alice:rwxadtTc", // r,w,x,p,d,a,A,c in canonical rwxaDdtTnNcCoy order
		},
		{
			"shorthand-read-only",
			ACE{Type: ACETypeAllow, Principal: "EVERYONE@", Permissions: []ACLPerm{PermReadOnly}},
			"A::EVERYONE@:rxtc",
		},
		{
			"duplicate-perms-deduped",
			ACE{Type: ACETypeAllow, Principal: "OWNER@", Permissions: []ACLPerm{PermRead, PermRead, PermWrite}},
			"A::OWNER@:rw",
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := aceToString(c.ace)
			if err != nil {
				t.Fatalf("aceToString: %v", err)
			}
			if got != c.want {
				t.Errorf("got %q want %q", got, c.want)
			}
		})
	}
}

// "modify" must include execute (NTFS Modify includes Read & Execute).
// Verify the literal letters explicitly.
func TestACEToString_ModifyIncludesExecute(t *testing.T) {
	got, err := aceToString(ACE{
		Type: ACETypeAllow, Principal: "user:alice",
		Permissions: []ACLPerm{PermModify},
	})
	if err != nil {
		t.Fatal(err)
	}
	// Canonical order: rwxaDdtTnNcCoy. Modify is r+w+x+p+d+a+A+c → "rwxadtTc".
	if !strings.Contains(got, "x") {
		t.Errorf("modify shorthand must include execute, got %q", got)
	}
	if got != "A::alice:rwxadtTc" {
		t.Errorf("modify expansion changed: got %q want %q", got, "A::alice:rwxadtTc")
	}
}

func TestACEToString_RejectsBad(t *testing.T) {
	cases := []struct {
		name string
		ace  ACE
	}{
		{"empty-type", ACE{Principal: "OWNER@", Permissions: []ACLPerm{PermRead}}},
		{"empty-principal", ACE{Type: ACETypeAllow, Permissions: []ACLPerm{PermRead}}},
		{"empty-perms", ACE{Type: ACETypeAllow, Principal: "OWNER@"}},
		{"unknown-perm", ACE{Type: ACETypeAllow, Principal: "OWNER@",
			Permissions: []ACLPerm{ACLPerm("nuke")}}},
		{"shorthand-plus-specific", ACE{Type: ACETypeAllow, Principal: "OWNER@",
			Permissions: []ACLPerm{PermReadOnly, PermRead}}},
		{"two-shorthands", ACE{Type: ACETypeAllow, Principal: "OWNER@",
			Permissions: []ACLPerm{PermReadOnly, PermModify}}},
		{"lowercase-special", ACE{Type: ACETypeAllow, Principal: "owner@",
			Permissions: []ACLPerm{PermRead}}},
		{"unknown-inherit", ACE{Type: ACETypeAllow, Principal: "OWNER@",
			Permissions: []ACLPerm{PermRead}, Inheritance: []InheritFlag{InheritFlag("forever")}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := aceToString(c.ace); err == nil {
				t.Errorf("expected error")
			}
		})
	}
}

func TestParseACE(t *testing.T) {
	cases := []struct {
		name string
		line string
		want ACE
	}{
		{
			"allow-owner-rwx",
			"A::OWNER@:rwx",
			ACE{Type: ACETypeAllow, Principal: "OWNER@",
				Permissions: []ACLPerm{PermRead, PermWrite, PermExecute}},
		},
		{
			"allow-group-with-flags",
			"A:fdg:GROUP@:rxc",
			ACE{Type: ACETypeAllow, Principal: "GROUP@",
				Permissions: []ACLPerm{PermRead, PermExecute, PermReadACL},
				Inheritance: []InheritFlag{InheritFile, InheritDir}},
		},
		{
			"deny-user",
			"D::alice:w",
			ACE{Type: ACETypeDeny, Principal: "user:alice",
				Permissions: []ACLPerm{PermWrite}},
		},
		{
			"group-from-g-flag",
			"A:g:eng@:r",
			ACE{Type: ACETypeAllow, Principal: "group:eng",
				Permissions: []ACLPerm{PermRead}},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseACE(c.line)
			if err != nil {
				t.Fatal(err)
			}
			if got.Type != c.want.Type {
				t.Errorf("Type: got %q want %q", got.Type, c.want.Type)
			}
			if got.Principal != c.want.Principal {
				t.Errorf("Principal: got %q want %q", got.Principal, c.want.Principal)
			}
			if !permSetsEqual(got.Permissions, c.want.Permissions) {
				t.Errorf("Permissions: got %v want %v", got.Permissions, c.want.Permissions)
			}
			if !inheritSetsEqual(got.Inheritance, c.want.Inheritance) {
				t.Errorf("Inheritance: got %v want %v", got.Inheritance, c.want.Inheritance)
			}
		})
	}
}

func TestParseACE_Errors(t *testing.T) {
	cases := []struct {
		name string
		in   string
	}{
		{"too-few-fields", "A::OWNER@"},
		{"unknown-type", "X::OWNER@:r"},
		{"audit-type", "U::OWNER@:r"},
		{"unknown-flag", "A:Z:OWNER@:r"},
		{"unknown-perm", "A::OWNER@:rZ"},
		{"empty-perms", "A::OWNER@:"},
		{"empty-principal", "A:::r"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if _, err := parseACE(c.in); err == nil {
				t.Errorf("expected error for %q", c.in)
			}
		})
	}
}

// Round-trip: aceToString(parseACE(line)) is equivalent to the original
// (allowing perm-letter reordering).
func TestACERoundTrip(t *testing.T) {
	lines := []string{
		"A::OWNER@:rwx",
		"D::EVERYONE@:w",
		"A:fd:alice:rwa",
		"A:g:eng@:r",
		"A:fdg:GROUP@:rxc",
		"A:i:user@DOMAIN:rwxaDdtTnNcCoy",
	}
	for _, line := range lines {
		t.Run(line, func(t *testing.T) {
			ace, err := parseACE(line)
			if err != nil {
				t.Fatalf("parseACE: %v", err)
			}
			got, err := aceToString(ace)
			if err != nil {
				t.Fatalf("aceToString: %v", err)
			}
			if !aceLineEquivalent(line, got) {
				t.Errorf("round trip:\n  in:  %q\n  out: %q", line, got)
			}
		})
	}
}

// aceLineEquivalent compares two wire-format ACE lines, treating the perm
// and flag substrings as sets.
func aceLineEquivalent(a, b string) bool {
	pa := strings.SplitN(a, ":", 4)
	pb := strings.SplitN(b, ":", 4)
	if len(pa) != 4 || len(pb) != 4 {
		return false
	}
	if pa[0] != pb[0] || pa[2] != pb[2] {
		return false
	}
	return sortBytes(pa[1]) == sortBytes(pb[1]) && sortBytes(pa[3]) == sortBytes(pb[3])
}

func sortBytes(s string) string {
	b := []byte(s)
	sort.Slice(b, func(i, j int) bool { return b[i] < b[j] })
	return string(b)
}

func permSetsEqual(a, b []ACLPerm) bool {
	if len(a) != len(b) {
		return false
	}
	ma := make(map[ACLPerm]int, len(a))
	for _, p := range a {
		ma[p]++
	}
	for _, p := range b {
		ma[p]--
	}
	for _, v := range ma {
		if v != 0 {
			return false
		}
	}
	return true
}

func inheritSetsEqual(a, b []InheritFlag) bool {
	if len(a) != len(b) {
		return false
	}
	ma := make(map[InheritFlag]int, len(a))
	for _, p := range a {
		ma[p]++
	}
	for _, p := range b {
		ma[p]--
	}
	for _, v := range ma {
		if v != 0 {
			return false
		}
	}
	return true
}

// --- Manager-level tests with a fake Runner -------------------------------

// fakeCall records one invocation of the fake runner.
type fakeCall struct {
	bin  string
	args []string
}

// fakeRunner returns canned stdout/err and records every call.
type fakeRunner struct {
	calls  []fakeCall
	stdout []byte
	err    error
}

func (f *fakeRunner) run(_ context.Context, bin string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, fakeCall{bin: bin, args: append([]string(nil), args...)})
	return f.stdout, f.err
}

func TestManager_GetACL(t *testing.T) {
	out := []byte(`# file: /tank/share
# owner: root
# group: root
A::OWNER@:rwx
A:fdg:GROUP@:rx
A::EVERYONE@:r
`)
	fr := &fakeRunner{stdout: out}
	m := &Manager{Runner: fr.run}
	aces, err := m.GetACL(context.Background(), "/tank/share")
	if err != nil {
		t.Fatal(err)
	}
	if len(aces) != 3 {
		t.Fatalf("want 3 ACEs, got %d", len(aces))
	}
	if aces[0].Principal != "OWNER@" || aces[1].Principal != "GROUP@" || aces[2].Principal != "EVERYONE@" {
		t.Errorf("principals: %v %v %v", aces[0].Principal, aces[1].Principal, aces[2].Principal)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %d", len(fr.calls))
	}
	if fr.calls[0].bin != NFS4GetFACLBin {
		t.Errorf("bin: %q", fr.calls[0].bin)
	}
	if !reflect.DeepEqual(fr.calls[0].args, []string{"/tank/share"}) {
		t.Errorf("args: %v", fr.calls[0].args)
	}
}

func TestManager_GetACL_NotSupported(t *testing.T) {
	fr := &fakeRunner{
		err: &exec.HostError{
			Bin:      NFS4GetFACLBin,
			ExitCode: 1,
			Stderr:   "Operation not supported",
		},
	}
	m := &Manager{Runner: fr.run}
	_, err := m.GetACL(context.Background(), "/tank/share")
	if !errors.Is(err, ErrACLNotSupported) {
		t.Errorf("want ErrACLNotSupported, got %v", err)
	}
}

func TestManager_GetACL_RejectBadPath(t *testing.T) {
	m := &Manager{Runner: (&fakeRunner{}).run}
	if _, err := m.GetACL(context.Background(), "relative/path"); err == nil {
		t.Error("expected error")
	}
}

func TestManager_SetACL(t *testing.T) {
	fr := &fakeRunner{}
	m := &Manager{Runner: fr.run}
	aces := []ACE{
		{Type: ACETypeAllow, Principal: "OWNER@", Permissions: []ACLPerm{PermFullControl}},
		{Type: ACETypeAllow, Principal: "EVERYONE@", Permissions: []ACLPerm{PermReadOnly}},
	}
	if err := m.SetACL(context.Background(), "/tank/share", aces); err != nil {
		t.Fatal(err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %d", len(fr.calls))
	}
	c := fr.calls[0]
	if c.bin != NFS4SetFACLBin {
		t.Errorf("bin: %q", c.bin)
	}
	// argv should be: -S <tempfile> /tank/share
	if len(c.args) != 3 || c.args[0] != "-S" || c.args[2] != "/tank/share" {
		t.Errorf("args: %v", c.args)
	}
	if !strings.HasPrefix(c.args[1], "/") {
		t.Errorf("tempfile path not absolute: %q", c.args[1])
	}
}

func TestManager_SetACL_RejectEmpty(t *testing.T) {
	m := &Manager{Runner: (&fakeRunner{}).run}
	if err := m.SetACL(context.Background(), "/tank/share", nil); err == nil {
		t.Error("expected error: empty ACL")
	}
}

func TestManager_SetACL_RejectBadPath(t *testing.T) {
	m := &Manager{Runner: (&fakeRunner{}).run}
	aces := []ACE{{Type: ACETypeAllow, Principal: "OWNER@", Permissions: []ACLPerm{PermRead}}}
	if err := m.SetACL(context.Background(), "../etc", aces); err == nil {
		t.Error("expected error")
	}
}

func TestManager_SetACL_RejectBadACE(t *testing.T) {
	m := &Manager{Runner: (&fakeRunner{}).run}
	aces := []ACE{{Type: ACETypeAllow, Principal: "OWNER@", Permissions: []ACLPerm{ACLPerm("bogus")}}}
	if err := m.SetACL(context.Background(), "/tank/share", aces); err == nil {
		t.Error("expected error")
	}
}

func TestManager_SetACL_NotSupported(t *testing.T) {
	fr := &fakeRunner{
		err: &exec.HostError{
			Bin:      NFS4SetFACLBin,
			ExitCode: 1,
			Stderr:   "nfs4_setfacl: Operation not supported",
		},
	}
	m := &Manager{Runner: fr.run}
	aces := []ACE{{Type: ACETypeAllow, Principal: "OWNER@", Permissions: []ACLPerm{PermRead}}}
	err := m.SetACL(context.Background(), "/tank/share", aces)
	if !errors.Is(err, ErrACLNotSupported) {
		t.Errorf("want ErrACLNotSupported, got %v", err)
	}
}

func TestManager_AppendACE(t *testing.T) {
	fr := &fakeRunner{}
	m := &Manager{Runner: fr.run}
	ace := ACE{Type: ACETypeAllow, Principal: "user:bob", Permissions: []ACLPerm{PermRead}}
	if err := m.AppendACE(context.Background(), "/tank/share", ace); err != nil {
		t.Fatal(err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %d", len(fr.calls))
	}
	c := fr.calls[0]
	want := []string{"-a", "A::bob:r", "/tank/share"}
	if !reflect.DeepEqual(c.args, want) {
		t.Errorf("args: got %v want %v", c.args, want)
	}
}

func TestManager_RemoveACE(t *testing.T) {
	fr := &fakeRunner{}
	m := &Manager{Runner: fr.run}
	if err := m.RemoveACE(context.Background(), "/tank/share", 2); err != nil {
		t.Fatal(err)
	}
	if len(fr.calls) != 1 {
		t.Fatalf("calls: %d", len(fr.calls))
	}
	// 0-based 2 → 1-based 3
	want := []string{"-x", "3", "/tank/share"}
	if !reflect.DeepEqual(fr.calls[0].args, want) {
		t.Errorf("args: got %v want %v", fr.calls[0].args, want)
	}
}

func TestManager_RemoveACE_RejectNegative(t *testing.T) {
	m := &Manager{Runner: (&fakeRunner{}).run}
	if err := m.RemoveACE(context.Background(), "/tank/share", -1); err == nil {
		t.Error("expected error")
	}
}
