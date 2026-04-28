package dataset

import (
	"reflect"
	"strings"
	"testing"
)

// --- Rename ----------------------------------------------------------------

func TestBuildRenameArgs(t *testing.T) {
	cases := []struct {
		name      string
		old, new_ string
		recursive bool
		want      []string
	}{
		{"plain", "tank/a", "tank/b", false, []string{"rename", "tank/a", "tank/b"}},
		{"recursive", "tank/a", "tank/b", true, []string{"rename", "-r", "tank/a", "tank/b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildRenameArgs(c.old, c.new_, c.recursive)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestBuildRenameArgs_RejectLeadingDashOld(t *testing.T) {
	if _, err := buildRenameArgs("-evil", "tank/b", false); err == nil {
		t.Error("expected error for leading-dash old name")
	}
}

func TestBuildRenameArgs_RejectLeadingDashNew(t *testing.T) {
	if _, err := buildRenameArgs("tank/a", "-evil", false); err == nil {
		t.Error("expected error for leading-dash new name")
	}
}

func TestBuildRenameArgs_RejectBadName(t *testing.T) {
	if _, err := buildRenameArgs("tank/a", "tank/bad@name", false); err == nil {
		t.Error("expected error for invalid name")
	}
}

// --- Clone -----------------------------------------------------------------

func TestBuildCloneArgs(t *testing.T) {
	got, err := buildCloneArgs("tank/a@snap1", "tank/b", map[string]string{
		"compression": "lz4",
		"atime":       "off",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"clone", "-o", "atime=off", "-o", "compression=lz4", "tank/a@snap1", "tank/b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildCloneArgs_NoProps(t *testing.T) {
	got, err := buildCloneArgs("tank/a@s", "tank/b", nil)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"clone", "tank/a@s", "tank/b"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildCloneArgs_RejectBadSnapshot(t *testing.T) {
	if _, err := buildCloneArgs("tank/a", "tank/b", nil); err == nil {
		t.Error("expected error: missing @ in snapshot name")
	}
}

// --- Promote ---------------------------------------------------------------

func TestBuildPromoteArgs(t *testing.T) {
	got, err := buildPromoteArgs("tank/a")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"promote", "tank/a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildPromoteArgs_RejectBadName(t *testing.T) {
	if _, err := buildPromoteArgs("tank/a@snap"); err == nil {
		t.Error("expected error for snapshot-style name")
	}
}

// --- LoadKey ---------------------------------------------------------------

func TestBuildLoadKeyArgs(t *testing.T) {
	cases := []struct {
		name        string
		ds          string
		keylocation string
		recursive   bool
		want        []string
	}{
		{"plain", "tank/a", "", false, []string{"load-key", "tank/a"}},
		{"with-keylocation", "tank/a", "file:///etc/key", false,
			[]string{"load-key", "-L", "file:///etc/key", "tank/a"}},
		{"recursive", "tank/a", "", true, []string{"load-key", "-r", "tank/a"}},
		{"recursive-with-keylocation", "tank/a", "prompt", true,
			[]string{"load-key", "-r", "-L", "prompt", "tank/a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildLoadKeyArgs(c.ds, c.keylocation, c.recursive)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestBuildLoadKeyArgs_RejectBadName(t *testing.T) {
	if _, err := buildLoadKeyArgs("bad@name", "", false); err == nil {
		t.Error("expected error for invalid name")
	}
}

// --- UnloadKey -------------------------------------------------------------

func TestBuildUnloadKeyArgs(t *testing.T) {
	cases := []struct {
		name      string
		ds        string
		recursive bool
		want      []string
	}{
		{"plain", "tank/a", false, []string{"unload-key", "tank/a"}},
		{"recursive", "tank/a", true, []string{"unload-key", "-r", "tank/a"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildUnloadKeyArgs(c.ds, c.recursive)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestBuildUnloadKeyArgs_RejectBadName(t *testing.T) {
	if _, err := buildUnloadKeyArgs("bad@name", false); err == nil {
		t.Error("expected error for invalid name")
	}
}

// --- ChangeKey -------------------------------------------------------------

func TestBuildChangeKeyArgs(t *testing.T) {
	got, err := buildChangeKeyArgs("tank/a", map[string]string{
		"keyformat":   "passphrase",
		"keylocation": "prompt",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"change-key", "-o", "keyformat=passphrase", "-o", "keylocation=prompt", "tank/a"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildChangeKeyArgs_RejectBadName(t *testing.T) {
	if _, err := buildChangeKeyArgs("bad@name", nil); err == nil {
		t.Error("expected error for invalid name")
	}
}

// --- Send ------------------------------------------------------------------

func TestBuildSendArgs_Ordering(t *testing.T) {
	// Verifies emission order: -R -w -c -L -e -i <from> <snapshot>
	got, err := buildSendArgs("tank/a@s2", SendOpts{
		Recursive:       true,
		Raw:             true,
		Compressed:      true,
		LargeBlock:      true,
		EmbeddedData:    true,
		IncrementalFrom: "tank/a@s1",
	})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"send", "-R", "-w", "-c", "-L", "-e", "-i", "tank/a@s1", "tank/a@s2"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildSendArgs_NoFlags(t *testing.T) {
	got, err := buildSendArgs("tank/a@s", SendOpts{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"send", "tank/a@s"}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %v want %v", got, want)
	}
}

func TestBuildSendArgs_NoIncrementalWhenEmpty(t *testing.T) {
	got, err := buildSendArgs("tank/a@s", SendOpts{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	for _, a := range got {
		if a == "-i" {
			t.Errorf("unexpected -i in %v", got)
		}
	}
	if got[len(got)-1] != "tank/a@s" {
		t.Errorf("snapshot must be last, got %v", got)
	}
}

func TestBuildSendArgs_RejectBadSnapshot(t *testing.T) {
	if _, err := buildSendArgs("tank/a", SendOpts{}); err == nil {
		t.Error("expected error: missing @")
	}
}

func TestBuildSendArgs_RejectBadIncremental(t *testing.T) {
	_, err := buildSendArgs("tank/a@s2", SendOpts{IncrementalFrom: "tank/a@-bad"})
	if err == nil {
		t.Error("expected error for invalid incremental snapshot")
	}
	if !strings.Contains(err.Error(), "incremental") {
		t.Errorf("error should mention incremental: %v", err)
	}
}

// --- Receive ---------------------------------------------------------------

func TestBuildReceiveArgs(t *testing.T) {
	cases := []struct {
		name   string
		target string
		opts   RecvOpts
		want   []string
	}{
		{"plain", "tank/b", RecvOpts{}, []string{"receive", "tank/b"}},
		{"force", "tank/b", RecvOpts{Force: true}, []string{"receive", "-F", "tank/b"}},
		{"resumable", "tank/b", RecvOpts{Resumable: true}, []string{"receive", "-s", "tank/b"}},
		{"force+resumable", "tank/b", RecvOpts{Force: true, Resumable: true},
			[]string{"receive", "-F", "-s", "tank/b"}},
		{"origin", "tank/b", RecvOpts{OriginSnapshot: "tank/a@s"},
			[]string{"receive", "-o", "origin=tank/a@s", "tank/b"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := buildReceiveArgs(c.target, c.opts)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("got %v want %v", got, c.want)
			}
		})
	}
}

func TestBuildReceiveArgs_RejectBadTarget(t *testing.T) {
	if _, err := buildReceiveArgs("bad@name", RecvOpts{}); err == nil {
		t.Error("expected error for invalid target")
	}
}

func TestBuildReceiveArgs_RejectBadOrigin(t *testing.T) {
	if _, err := buildReceiveArgs("tank/b", RecvOpts{OriginSnapshot: "no-at-sign"}); err == nil {
		t.Error("expected error for invalid origin snapshot")
	}
}
