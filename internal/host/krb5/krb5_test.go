package krb5

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"strings"
	"testing"
	"time"
)

// memFS is an in-memory FileSystem for tests.
type memFS struct {
	files map[string][]byte
	perms map[string]os.FileMode
}

func newMemFS() *memFS {
	return &memFS{files: map[string][]byte{}, perms: map[string]os.FileMode{}}
}

func (m *memFS) ReadFile(p string) ([]byte, error) {
	d, ok := m.files[p]
	if !ok {
		return nil, &fs.PathError{Op: "open", Path: p, Err: fs.ErrNotExist}
	}
	out := make([]byte, len(d))
	copy(out, d)
	return out, nil
}
func (m *memFS) WriteFile(p string, d []byte, perm os.FileMode) error {
	cp := make([]byte, len(d))
	copy(cp, d)
	m.files[p] = cp
	m.perms[p] = perm
	return nil
}
func (m *memFS) Stat(p string) (os.FileInfo, error) {
	if _, ok := m.files[p]; !ok {
		return nil, &fs.PathError{Op: "stat", Path: p, Err: fs.ErrNotExist}
	}
	return memInfo{name: p, size: int64(len(m.files[p]))}, nil
}
func (m *memFS) Remove(p string) error {
	if _, ok := m.files[p]; !ok {
		return &fs.PathError{Op: "remove", Path: p, Err: fs.ErrNotExist}
	}
	delete(m.files, p)
	delete(m.perms, p)
	return nil
}

type memInfo struct {
	name string
	size int64
}

func (i memInfo) Name() string       { return i.name }
func (i memInfo) Size() int64        { return i.size }
func (i memInfo) Mode() os.FileMode  { return 0o600 }
func (i memInfo) ModTime() time.Time { return time.Time{} }
func (i memInfo) IsDir() bool        { return false }
func (i memInfo) Sys() any           { return nil }

// --- krb5.conf round-trip -------------------------------------------------

func TestKrb5ConfRoundTrip(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{
			name: "single-realm",
			cfg: Config{
				DefaultRealm: "EXAMPLE.COM",
				Realms: map[string]Realm{
					"EXAMPLE.COM": {KDC: []string{"kdc1.example.com", "kdc2.example.com:88"}},
				},
			},
		},
		{
			name: "with-admin-and-default-domain",
			cfg: Config{
				DefaultRealm:   "EXAMPLE.COM",
				DNSLookupKDC:   true,
				DNSLookupRealm: false,
				Realms: map[string]Realm{
					"EXAMPLE.COM": {
						KDC:           []string{"kdc.example.com"},
						AdminServer:   "kadmin.example.com:749",
						DefaultDomain: "example.com",
					},
				},
				DomainRealm: map[string]string{
					"example.com":  "EXAMPLE.COM",
					".example.com": "EXAMPLE.COM",
				},
			},
		},
		{
			name: "multi-realm",
			cfg: Config{
				DefaultRealm: "A.EXAMPLE.COM",
				Realms: map[string]Realm{
					"A.EXAMPLE.COM": {KDC: []string{"kdc.a.example.com"}},
					"B.EXAMPLE.COM": {KDC: []string{"kdc.b.example.com"}},
				},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r1 := renderKrb5Conf(tc.cfg)
			parsed, err := parseKrb5Conf(r1)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			r2 := renderKrb5Conf(*parsed)
			if !bytes.Equal(r1, r2) {
				t.Errorf("round-trip mismatch\nfirst:\n%s\nsecond:\n%s", r1, r2)
			}
		})
	}
}

// --- idmapd.conf round-trip -----------------------------------------------

func TestIdmapdConfRoundTrip(t *testing.T) {
	cases := []IdmapdConfig{
		{Domain: "example.com"},
		{Domain: "corp.example.com", Verbosity: 4},
	}
	for _, cfg := range cases {
		r1 := renderIdmapdConf(cfg)
		parsed, err := parseIdmapdConf(r1)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		r2 := renderIdmapdConf(*parsed)
		if !bytes.Equal(r1, r2) {
			t.Errorf("round-trip mismatch for %+v\nfirst:\n%s\nsecond:\n%s", cfg, r1, r2)
		}
	}
}

// --- klist parsing --------------------------------------------------------

func TestParseKlistOutput(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []KeytabEntry
	}{
		{
			name: "empty",
			in:   "",
			want: []KeytabEntry{},
		},
		{
			name: "whitespace-only",
			in:   "   \n\n",
			want: []KeytabEntry{},
		},
		{
			name: "single-entry",
			in: `Keytab name: FILE:/etc/krb5.keytab
KVNO Timestamp           Principal
---- ------------------- ---------------------------------------------------------
   2 04/29/2026 06:18:00 nfs/host.example.com@EXAMPLE.COM (aes256-cts-hmac-sha1-96)
`,
			want: []KeytabEntry{
				{KVNO: 2, Principal: "nfs/host.example.com@EXAMPLE.COM", Encryption: "aes256-cts-hmac-sha1-96"},
			},
		},
		{
			name: "multiple-entries",
			in: `Keytab name: FILE:/etc/krb5.keytab
KVNO Timestamp           Principal
---- ------------------- ---------------------------------------------------------
   3 04/29/2026 06:18:00 host/host.example.com@EXAMPLE.COM (aes256-cts-hmac-sha1-96)
   3 04/29/2026 06:18:00 host/host.example.com@EXAMPLE.COM (aes128-cts-hmac-sha1-96)
   5 04/29/2026 06:18:00 nfs/host.example.com@EXAMPLE.COM (aes256-cts-hmac-sha1-96)
`,
			want: []KeytabEntry{
				{KVNO: 3, Principal: "host/host.example.com@EXAMPLE.COM", Encryption: "aes256-cts-hmac-sha1-96"},
				{KVNO: 3, Principal: "host/host.example.com@EXAMPLE.COM", Encryption: "aes128-cts-hmac-sha1-96"},
				{KVNO: 5, Principal: "nfs/host.example.com@EXAMPLE.COM", Encryption: "aes256-cts-hmac-sha1-96"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := parseKlistOutput([]byte(tc.in))
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(got) != len(tc.want) {
				t.Fatalf("got %d entries, want %d: %+v", len(got), len(tc.want), got)
			}
			for i := range got {
				if got[i] != tc.want[i] {
					t.Errorf("entry %d: got %+v want %+v", i, got[i], tc.want[i])
				}
			}
		})
	}
}

// --- UploadKeytab ---------------------------------------------------------

func TestUploadKeytab(t *testing.T) {
	cases := []struct {
		name    string
		data    []byte
		wantErr bool
	}{
		{name: "empty", data: nil, wantErr: true},
		{name: "wrong-magic", data: []byte{0x06, 0x02, 0x00}, wantErr: true},
		{name: "ascii", data: []byte("not a keytab"), wantErr: true},
		{name: "valid-magic-v2", data: []byte{0x05, 0x02, 0xde, 0xad}, wantErr: false},
		{name: "valid-magic-v1", data: []byte{0x05, 0x01, 0xbe, 0xef}, wantErr: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fs := newMemFS()
			m := &Manager{KeytabPath: "/etc/krb5.keytab", FS: fs}
			err := m.UploadKeytab(context.Background(), tc.data)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, tc.wantErr)
			}
			if !tc.wantErr {
				got, ok := fs.files["/etc/krb5.keytab"]
				if !ok {
					t.Fatal("file not written")
				}
				if !bytes.Equal(got, tc.data) {
					t.Errorf("contents mismatch")
				}
				if fs.perms["/etc/krb5.keytab"] != 0o600 {
					t.Errorf("perm = %o want 0600", fs.perms["/etc/krb5.keytab"])
				}
			}
		})
	}
}

func TestDeleteKeytabIdempotent(t *testing.T) {
	fs := newMemFS()
	m := &Manager{KeytabPath: "/etc/krb5.keytab", FS: fs}
	// missing file: no error
	if err := m.DeleteKeytab(context.Background()); err != nil {
		t.Fatalf("missing: %v", err)
	}
	// existing: removed
	_ = fs.WriteFile("/etc/krb5.keytab", []byte{0x05, 0x02}, 0o600)
	if err := m.DeleteKeytab(context.Background()); err != nil {
		t.Fatalf("existing: %v", err)
	}
	if _, ok := fs.files["/etc/krb5.keytab"]; ok {
		t.Fatal("file still present")
	}
}

// --- GetConfig / SetConfig ------------------------------------------------

func TestGetConfigMissing(t *testing.T) {
	m := &Manager{Krb5ConfPath: "/etc/krb5.conf", FS: newMemFS()}
	cfg, err := m.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if cfg.DefaultRealm != "" || len(cfg.Realms) != 0 {
		t.Errorf("expected zero-value config, got %+v", cfg)
	}
}

func TestSetGetConfig(t *testing.T) {
	fs := newMemFS()
	m := &Manager{Krb5ConfPath: "/etc/krb5.conf", FS: fs}
	in := Config{
		DefaultRealm: "EXAMPLE.COM",
		DNSLookupKDC: true,
		Realms: map[string]Realm{
			"EXAMPLE.COM": {KDC: []string{"kdc.example.com"}, AdminServer: "kadmin.example.com"},
		},
	}
	if err := m.SetConfig(context.Background(), in); err != nil {
		t.Fatalf("set: %v", err)
	}
	if fs.perms["/etc/krb5.conf"] != 0o644 {
		t.Errorf("perm = %o want 0644", fs.perms["/etc/krb5.conf"])
	}
	out, err := m.GetConfig(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if out.DefaultRealm != in.DefaultRealm || !out.DNSLookupKDC {
		t.Errorf("mismatch: %+v", out)
	}
	r := out.Realms["EXAMPLE.COM"]
	if len(r.KDC) != 1 || r.KDC[0] != "kdc.example.com" || r.AdminServer != "kadmin.example.com" {
		t.Errorf("realm mismatch: %+v", r)
	}
}

// --- Validation negatives -------------------------------------------------

func TestValidateConfig(t *testing.T) {
	cases := []struct {
		name string
		cfg  Config
	}{
		{name: "empty-default-realm", cfg: Config{Realms: map[string]Realm{}}},
		{
			name: "lowercase-realm",
			cfg: Config{
				DefaultRealm: "example.com",
				Realms:       map[string]Realm{"example.com": {KDC: []string{"k"}}},
			},
		},
		{
			name: "default-not-in-realms",
			cfg: Config{
				DefaultRealm: "EXAMPLE.COM",
				Realms:       map[string]Realm{"OTHER.COM": {KDC: []string{"k.other.com"}}},
			},
		},
		{
			name: "no-kdc",
			cfg: Config{
				DefaultRealm: "EXAMPLE.COM",
				Realms:       map[string]Realm{"EXAMPLE.COM": {}},
			},
		},
		{
			name: "bad-kdc-port",
			cfg: Config{
				DefaultRealm: "EXAMPLE.COM",
				Realms:       map[string]Realm{"EXAMPLE.COM": {KDC: []string{"kdc.example.com:99999"}}},
			},
		},
		{
			name: "bad-admin-server",
			cfg: Config{
				DefaultRealm: "EXAMPLE.COM",
				Realms: map[string]Realm{
					"EXAMPLE.COM": {KDC: []string{"k.example.com"}, AdminServer: "not a host"},
				},
			},
		},
		{
			name: "bad-domain-realm-key",
			cfg: Config{
				DefaultRealm: "EXAMPLE.COM",
				Realms:       map[string]Realm{"EXAMPLE.COM": {KDC: []string{"k.example.com"}}},
				DomainRealm:  map[string]string{"not a dns!": "EXAMPLE.COM"},
			},
		},
		{
			name: "bad-domain-realm-value-lowercase",
			cfg: Config{
				DefaultRealm: "EXAMPLE.COM",
				Realms:       map[string]Realm{"EXAMPLE.COM": {KDC: []string{"k.example.com"}}},
				DomainRealm:  map[string]string{"example.com": "example.com"},
			},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := validateConfig(tc.cfg); err == nil {
				t.Errorf("expected error, got nil")
			}
		})
	}
}

func TestValidateConfigOK(t *testing.T) {
	cfg := Config{
		DefaultRealm: "EXAMPLE.COM",
		Realms: map[string]Realm{
			"EXAMPLE.COM": {
				KDC:         []string{"kdc.example.com", "10.0.0.1:88", "[2001:db8::1]:88"},
				AdminServer: "kadmin.example.com:749",
			},
		},
		DomainRealm: map[string]string{
			"example.com":  "EXAMPLE.COM",
			".example.com": "EXAMPLE.COM",
		},
	}
	if err := validateConfig(cfg); err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestValidateIdmapd(t *testing.T) {
	bad := []IdmapdConfig{
		{},                                  // empty domain
		{Domain: "not a host!"},             // invalid DNS
		{Domain: "example.com", Verbosity: -1},
		{Domain: "example.com", Verbosity: 10},
	}
	for i, c := range bad {
		if err := validateIdmapd(c); err == nil {
			t.Errorf("case %d: expected error for %+v", i, c)
		}
	}
	if err := validateIdmapd(IdmapdConfig{Domain: "example.com", Verbosity: 3}); err != nil {
		t.Errorf("good case: %v", err)
	}
}

// --- ListKeytab / SetIdmapdConfig integration via mocks -------------------

func TestListKeytabMissingFile(t *testing.T) {
	m := &Manager{KeytabPath: "/etc/krb5.keytab", FS: newMemFS()}
	out, err := m.ListKeytab(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 0 {
		t.Errorf("expected empty, got %v", out)
	}
}

func TestListKeytabWithRunner(t *testing.T) {
	mfs := newMemFS()
	_ = mfs.WriteFile("/etc/krb5.keytab", []byte{0x05, 0x02}, 0o600)
	m := &Manager{
		KeytabPath: "/etc/krb5.keytab",
		KlistBin:   "/usr/bin/klist",
		FS:         mfs,
		Runner: func(ctx context.Context, bin string, args ...string) ([]byte, error) {
			if bin != "/usr/bin/klist" {
				return nil, errors.New("unexpected bin")
			}
			return []byte(`Keytab name: FILE:/etc/krb5.keytab
KVNO Timestamp           Principal
---- ------------------- ---------------------------------------------------------
   1 04/29/2026 06:18:00 nfs/host@EXAMPLE.COM (aes256-cts-hmac-sha1-96)
`), nil
		},
	}
	out, err := m.ListKeytab(context.Background())
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if len(out) != 1 || out[0].Principal != "nfs/host@EXAMPLE.COM" || out[0].KVNO != 1 {
		t.Errorf("got %+v", out)
	}
}

func TestSetIdmapdConfig(t *testing.T) {
	mfs := newMemFS()
	m := &Manager{IdmapdConfPath: "/etc/idmapd.conf", FS: mfs}
	if err := m.SetIdmapdConfig(context.Background(), IdmapdConfig{Domain: "example.com", Verbosity: 2}); err != nil {
		t.Fatalf("set: %v", err)
	}
	got := string(mfs.files["/etc/idmapd.conf"])
	if !strings.Contains(got, "[General]") || !strings.Contains(got, "Domain = example.com") {
		t.Errorf("rendered file missing expected sections:\n%s", got)
	}
	if !strings.Contains(got, "[Mapping]") {
		t.Errorf("missing [Mapping] section:\n%s", got)
	}
	if mfs.perms["/etc/idmapd.conf"] != 0o644 {
		t.Errorf("perm = %o", mfs.perms["/etc/idmapd.conf"])
	}
	parsed, err := m.GetIdmapdConfig(context.Background())
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if parsed.Domain != "example.com" || parsed.Verbosity != 2 {
		t.Errorf("parsed mismatch: %+v", parsed)
	}
}

// --- idmapd Translation/Mapping defaults ----------------------------------

func TestRenderIdmapdConfDefaults(t *testing.T) {
	got := string(renderIdmapdConf(IdmapdConfig{Domain: "novanas.local"}))
	wants := []string{
		"[General]",
		"Verbosity = 0",
		"Domain = novanas.local",
		"[Translation]",
		"Method = nsswitch",
		"[Mapping]",
		"Nobody-User = nobody",
		"Nobody-Group = nogroup",
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestRenderIdmapdConfOverrides(t *testing.T) {
	got := string(renderIdmapdConf(IdmapdConfig{
		Domain:      "novanas.local",
		Method:      "static",
		NobodyUser:  "nfsnobody",
		NobodyGroup: "nfsnobody",
	}))
	for _, w := range []string{"Method = static", "Nobody-User = nfsnobody", "Nobody-Group = nfsnobody"} {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestParseIdmapdConfTranslationAndMapping(t *testing.T) {
	body := `[General]
Verbosity = 1
Domain = novanas.local

[Translation]
Method = nsswitch

[Mapping]
Nobody-User = nobody
Nobody-Group = nogroup
`
	cfg, err := parseIdmapdConf([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if cfg.Domain != "novanas.local" || cfg.Verbosity != 1 {
		t.Errorf("general mismatch: %+v", cfg)
	}
	if cfg.Method != "nsswitch" {
		t.Errorf("Method = %q, want nsswitch", cfg.Method)
	}
	if cfg.NobodyUser != "nobody" || cfg.NobodyGroup != "nogroup" {
		t.Errorf("nobody mismatch: %+v", cfg)
	}
}

// --- rpc.gssd defaults ----------------------------------------------------

func TestRenderGssdDefaultsMachineCreds(t *testing.T) {
	got := string(renderGssdDefaults(GssdDefaults{Enabled: true, Args: "-n"}))
	wants := []string{
		"NEED_GSSD=yes",
		"GSS_USE_PROXY=no",
		`GSSDARGS="-n"`,
	}
	for _, w := range wants {
		if !strings.Contains(got, w) {
			t.Errorf("missing %q in:\n%s", w, got)
		}
	}
}

func TestRenderGssdDefaultsDisabled(t *testing.T) {
	got := string(renderGssdDefaults(GssdDefaults{}))
	if !strings.Contains(got, "NEED_GSSD=no") {
		t.Errorf("expected NEED_GSSD=no in:\n%s", got)
	}
	if !strings.Contains(got, `GSSDARGS=""`) {
		t.Errorf("expected empty GSSDARGS in:\n%s", got)
	}
}

func TestValidateGssdDefaultsRejectsShellMeta(t *testing.T) {
	bad := []string{`-n; rm -rf /`, "-n `whoami`", `-n "$(id)"`, `-n\nfoo`}
	for _, a := range bad {
		if err := validateGssdDefaults(GssdDefaults{Args: a}); err == nil {
			t.Errorf("expected error for %q", a)
		}
	}
	good := []string{"", "-n", "-n -f", "-v -n", "-d /var/lib/nfs/rpc_pipefs"}
	for _, a := range good {
		if err := validateGssdDefaults(GssdDefaults{Args: a}); err != nil {
			t.Errorf("good args %q: %v", a, err)
		}
	}
}

func TestSetGssdDefaultsWritesFile(t *testing.T) {
	mfs := newMemFS()
	m := &Manager{NfsCommonPath: "/etc/default/nfs-common", FS: mfs}
	if err := m.SetGssdDefaults(context.Background(), GssdDefaults{Enabled: true, Args: "-n"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	body, ok := mfs.files["/etc/default/nfs-common"]
	if !ok {
		t.Fatal("file not written")
	}
	if !strings.Contains(string(body), `GSSDARGS="-n"`) {
		t.Errorf("body missing GSSDARGS:\n%s", body)
	}
	if mfs.perms["/etc/default/nfs-common"] != 0o644 {
		t.Errorf("perm = %o", mfs.perms["/etc/default/nfs-common"])
	}
}

func TestGssdDefaultsPathDefault(t *testing.T) {
	m := &Manager{}
	if got := m.GssdDefaultsPath(); got != "/etc/default/nfs-common" {
		t.Errorf("got %q, want /etc/default/nfs-common", got)
	}
}
