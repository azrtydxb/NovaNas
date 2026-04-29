package krb5

import (
	"context"
	"errors"
	"io/fs"
	"strings"
	"testing"
)

func TestValidatePrincipalName(t *testing.T) {
	good := []string{
		"nfs/host.example.com",
		"nfs/host.example.com@NOVANAS.LOCAL",
		"host/h1",
		"alice",
		"a_b-c.d/e",
	}
	for _, n := range good {
		if err := validatePrincipalName(n); err != nil {
			t.Errorf("good %q: %v", n, err)
		}
	}
	bad := []string{
		"",
		"foo bar",      // whitespace
		"foo;rm -rf /", // metachars
		"$(whoami)",
		"foo`bar`",
		"'quote'",
	}
	for _, n := range bad {
		if err := validatePrincipalName(n); err == nil {
			t.Errorf("bad %q: expected error", n)
		}
	}
}

func TestCreatePrincipal_RandkeyDefault(t *testing.T) {
	r := &fakeRunner{t: t, respond: func(bin string, args []string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "add_principal") {
			return nil, nil
		}
		// get_principal after add
		return []byte("Principal: nfs/host@NOVANAS.LOCAL\nKey version: vno 1\nExpiration date: never\nAttributes: \n"), nil
	}}
	m := &KDCManager{Runner: r.run, FS: newMemFS()}
	info, err := m.CreatePrincipal(context.Background(), CreatePrincipalSpec{Name: "nfs/host"})
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if info.Name != "nfs/host@NOVANAS.LOCAL" {
		t.Errorf("name=%q", info.Name)
	}
	// First call should be add_principal -randkey
	first := strings.Join(r.calls[0], " ")
	if !strings.Contains(first, "-randkey") {
		t.Errorf("expected -randkey in %q", first)
	}
}

func TestCreatePrincipal_PasswordPath(t *testing.T) {
	r := &fakeRunner{t: t, respond: func(bin string, args []string) ([]byte, error) {
		joined := strings.Join(args, " ")
		if strings.Contains(joined, "add_principal") {
			return nil, nil
		}
		return []byte("Principal: alice@NOVANAS.LOCAL\n"), nil
	}}
	m := &KDCManager{Runner: r.run, FS: newMemFS()}
	if _, err := m.CreatePrincipal(context.Background(), CreatePrincipalSpec{Name: "alice", Password: "p4ss"}); err != nil {
		t.Fatalf("create: %v", err)
	}
	first := strings.Join(r.calls[0], " ")
	if !strings.Contains(first, "-pw") {
		t.Errorf("expected -pw in %q", first)
	}
	if !strings.Contains(first, "p4ss") {
		t.Errorf("expected pw in %q", first)
	}
}

func TestCreatePrincipal_MutuallyExclusive(t *testing.T) {
	m := &KDCManager{Runner: (&fakeRunner{t: t}).run, FS: newMemFS()}
	_, err := m.CreatePrincipal(context.Background(), CreatePrincipalSpec{Name: "x", Randkey: true, Password: "p"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDeletePrincipal_IdempotentOnNotFound(t *testing.T) {
	r := &fakeRunner{t: t, respond: func(string, []string) ([]byte, error) {
		return nil, errors.New("Principal does not exist")
	}}
	m := &KDCManager{Runner: r.run, FS: newMemFS()}
	if err := m.DeletePrincipal(context.Background(), "nope"); err != nil {
		t.Fatalf("delete: %v", err)
	}
}

func TestGetPrincipal_NotFoundMapsToErrNotExist(t *testing.T) {
	r := &fakeRunner{t: t, respond: func(string, []string) ([]byte, error) {
		return nil, errors.New("get_principal: Principal does not exist while retrieving \"nope@X\"")
	}}
	m := &KDCManager{Runner: r.run, FS: newMemFS()}
	_, err := m.GetPrincipal(context.Background(), "nope")
	if !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("want fs.ErrNotExist, got %v", err)
	}
}

func TestListPrincipals_FiltersBanner(t *testing.T) {
	out := []byte(`Authenticating as principal root/admin@NOVANAS.LOCAL with password.
K/M@NOVANAS.LOCAL
krbtgt/NOVANAS.LOCAL@NOVANAS.LOCAL
nfs/host.example.com@NOVANAS.LOCAL
`)
	got := parseListPrincipals(out)
	if len(got) != 3 {
		t.Fatalf("got %d, want 3: %v", len(got), got)
	}
	if got[2] != "nfs/host.example.com@NOVANAS.LOCAL" {
		t.Errorf("got[2]=%q", got[2])
	}
}

func TestParseGetPrincipal(t *testing.T) {
	out := []byte(`Principal: nfs/h@R
Expiration date: [never]
Attributes: REQUIRES_PRE_AUTH
Key: vno 3, aes256-cts
`)
	info := parseGetPrincipal(out)
	if info == nil {
		t.Fatal("nil")
	}
	if info.Name != "nfs/h@R" {
		t.Errorf("name=%q", info.Name)
	}
	if info.Expiration != "[never]" {
		t.Errorf("exp=%q", info.Expiration)
	}
	if info.Attributes != "REQUIRES_PRE_AUTH" {
		t.Errorf("attr=%q", info.Attributes)
	}
}

func TestShellQuote(t *testing.T) {
	if got := shellQuote("a"); got != "'a'" {
		t.Errorf("got %q", got)
	}
	if got := shellQuote("a'b"); got != `'a'\''b'` {
		t.Errorf("got %q", got)
	}
}
