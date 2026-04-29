package krb5

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRunner records calls and returns canned responses keyed by binary
// + first arg (so e.g. "/bin/systemctl is-active" or "/usr/sbin/kadmin.local -q ...").
type fakeRunner struct {
	t       *testing.T
	calls   [][]string
	respond func(bin string, args []string) ([]byte, error)
}

func (f *fakeRunner) run(_ context.Context, bin string, args ...string) ([]byte, error) {
	f.calls = append(f.calls, append([]string{bin}, args...))
	if f.respond == nil {
		return nil, nil
	}
	return f.respond(bin, args)
}

func TestKDCStatus_DefaultsAndDBPresent(t *testing.T) {
	fs := newMemFS()
	// Pretend the database exists; stash absent.
	_ = fs.WriteFile("/var/lib/krb5kdc/principal", []byte{0x01}, 0o600)

	r := &fakeRunner{t: t, respond: func(bin string, args []string) ([]byte, error) {
		switch {
		case strings.HasSuffix(bin, "systemctl"):
			return []byte("active\n"), nil
		case strings.HasSuffix(bin, "kadmin.local"):
			// list_principals output
			return []byte("K/M@NOVANAS.LOCAL\nkrbtgt/NOVANAS.LOCAL@NOVANAS.LOCAL\n"), nil
		}
		return nil, nil
	}}
	m := &KDCManager{Runner: r.run, FS: fs}

	st, err := m.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.Realm != DefaultRealm {
		t.Errorf("realm=%q", st.Realm)
	}
	if !st.DatabaseExists {
		t.Errorf("expected DatabaseExists=true")
	}
	if st.StashSealed {
		t.Errorf("expected StashSealed=false")
	}
	if !st.Running {
		t.Errorf("expected Running=true")
	}
	if st.PrincipalCount != 2 {
		t.Errorf("PrincipalCount=%d want 2", st.PrincipalCount)
	}
}

func TestBootstrap_AlreadyExistsIsIdempotent(t *testing.T) {
	fs := newMemFS()
	_ = fs.WriteFile("/var/lib/krb5kdc/principal", []byte{0x01}, 0o600)
	r := &fakeRunner{t: t}
	m := &KDCManager{Runner: r.run, FS: fs}
	err := m.Bootstrap(context.Background(), "secret")
	if !IsAlreadyBootstrapped(err) {
		t.Fatalf("want already-bootstrapped, got %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("expected no exec calls, got %v", r.calls)
	}
}

func TestBootstrap_RunsKdb5UtilCreate(t *testing.T) {
	fs := newMemFS()
	r := &fakeRunner{t: t, respond: func(string, []string) ([]byte, error) { return nil, nil }}
	m := &KDCManager{Cfg: KDCConfig{Realm: "TEST.LOCAL"}, Runner: r.run, FS: fs}
	if err := m.Bootstrap(context.Background(), "hunter2"); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	if len(r.calls) != 1 {
		t.Fatalf("calls=%v", r.calls)
	}
	got := strings.Join(r.calls[0], " ")
	if !strings.Contains(got, "kdb5_util") {
		t.Errorf("want kdb5_util invocation, got %q", got)
	}
	if !strings.Contains(got, "TEST.LOCAL") {
		t.Errorf("want realm TEST.LOCAL, got %q", got)
	}
	if !strings.Contains(got, "create") || !strings.Contains(got, "-s") {
		t.Errorf("want create -s, got %q", got)
	}
	if !strings.Contains(got, "hunter2") {
		t.Errorf("want master pw passed, got %q", got)
	}
}

func TestBootstrap_RequiresPassword(t *testing.T) {
	m := &KDCManager{FS: newMemFS()}
	if err := m.Bootstrap(context.Background(), ""); err == nil {
		t.Fatal("expected error for empty password")
	}
}

func TestStatus_DBMissing(t *testing.T) {
	r := &fakeRunner{t: t, respond: func(bin string, args []string) ([]byte, error) {
		if strings.HasSuffix(bin, "systemctl") {
			return []byte("inactive\n"), errors.New("nonzero exit")
		}
		return nil, nil
	}}
	m := &KDCManager{Runner: r.run, FS: newMemFS()}
	st, err := m.Status(context.Background())
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if st.DatabaseExists || st.StashSealed || st.Running {
		t.Errorf("want all false, got %+v", st)
	}
}
