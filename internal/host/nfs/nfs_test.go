package nfs

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// ---------- shared fakes ----------

// captureRunner records argv from each invocation so tests can assert
// the exact exportfs call shape without executing anything.
type captureRunner struct {
	calls [][]string
	err   error
	out   []byte
}

func (c *captureRunner) run(_ context.Context, _ string, args ...string) ([]byte, error) {
	cp := append([]string(nil), args...)
	c.calls = append(c.calls, cp)
	return c.out, c.err
}

// captureFileWriter is an in-memory FileWriter that records every call.
type captureFileWriter struct {
	files       map[string][]byte
	writes      []writeCall
	removes     []string
	readErr     map[string]error
	writeErr    map[string]error
	dirEntries  []os.DirEntry
	readDirErr  error
	readDirPath string
}

type writeCall struct {
	Path string
	Data []byte
	Perm os.FileMode
}

func newCaptureFileWriter() *captureFileWriter {
	return &captureFileWriter{
		files:    map[string][]byte{},
		readErr:  map[string]error{},
		writeErr: map[string]error{},
	}
}

func (c *captureFileWriter) Write(path string, data []byte, perm os.FileMode) error {
	if err, ok := c.writeErr[path]; ok {
		return err
	}
	cp := append([]byte(nil), data...)
	c.writes = append(c.writes, writeCall{Path: path, Data: cp, Perm: perm})
	c.files[path] = cp
	return nil
}

func (c *captureFileWriter) Remove(path string) error {
	if _, ok := c.files[path]; !ok {
		return &os.PathError{Op: "remove", Path: path, Err: fs.ErrNotExist}
	}
	delete(c.files, path)
	c.removes = append(c.removes, path)
	return nil
}

func (c *captureFileWriter) ReadFile(path string) ([]byte, error) {
	if err, ok := c.readErr[path]; ok {
		return nil, err
	}
	if d, ok := c.files[path]; ok {
		return append([]byte(nil), d...), nil
	}
	return nil, &os.PathError{Op: "open", Path: path, Err: fs.ErrNotExist}
}

func (c *captureFileWriter) ReadDir(path string) ([]os.DirEntry, error) {
	c.readDirPath = path
	if c.readDirErr != nil {
		return nil, c.readDirErr
	}
	return c.dirEntries, nil
}

// stubDirEntry implements os.DirEntry for ReadDir fakes.
type stubDirEntry struct {
	name string
	dir  bool
}

func (s stubDirEntry) Name() string               { return s.name }
func (s stubDirEntry) IsDir() bool                { return s.dir }
func (s stubDirEntry) Type() os.FileMode          { return 0 }
func (s stubDirEntry) Info() (os.FileInfo, error) { return nil, nil }

func eq[T any](a, b T) bool { return reflect.DeepEqual(a, b) }

func newManager(fw FileWriter, r *captureRunner) *Manager {
	return &Manager{
		ExportsBin: "/usr/sbin/exportfs",
		ExportsDir: "/etc/exports.d",
		FilePrefix: "nova-nas-",
		Runner:     r.run,
		FileWriter: fw,
	}
}

func sampleExport() Export {
	return Export{
		Name: "share1",
		Path: "/tank/share1",
		Clients: []ClientRule{
			{Spec: "10.0.0.0/24", Options: "rw,sync,root_squash"},
			{Spec: "*", Options: "ro"},
		},
	}
}

// ---------- validators ----------

func TestValidateExportName(t *testing.T) {
	good := []string{"a", "share1", "share-1", "share_1", "AbC_-_123", strings.Repeat("a", 64)}
	bad := []string{"", "-bad", "has space", "has/slash", "has.dot", strings.Repeat("a", 65), "name!"}
	for _, n := range good {
		if err := validateExportName(n); err != nil {
			t.Errorf("good name %q: %v", n, err)
		}
	}
	for _, n := range bad {
		if err := validateExportName(n); err == nil {
			t.Errorf("bad name %q: expected error", n)
		}
	}
}

func TestValidateExportPath(t *testing.T) {
	good := []string{"/", "/tank", "/tank/share1", "/a/b-c_d.e"}
	bad := []string{
		"", "tank/share1", "/tank/../etc", "/tank/with space",
		"/tank/`whoami`", "/tank;rm", "/tank|x", "/tank$x", "/tank\x00",
		"/tank<x", "/tank*x", "/tank'x", "/tank\"x",
	}
	for _, p := range good {
		if err := validateExportPath(p); err != nil {
			t.Errorf("good path %q: %v", p, err)
		}
	}
	for _, p := range bad {
		if err := validateExportPath(p); err == nil {
			t.Errorf("bad path %q: expected error", p)
		}
	}
}

func TestValidateClientSpec(t *testing.T) {
	good := []string{"*", "10.0.0.0/24", "10.0.0.5", "::1", "fe80::/10", "host.example.com", "*.example.com", "h?st"}
	bad := []string{"", "10.0.0.0/33", "10.0.0.0/abc", "host with space", "-flag", "back`tick", "host;rm", "host(x)"}
	for _, s := range good {
		if err := validateClientSpec(s); err != nil {
			t.Errorf("good spec %q: %v", s, err)
		}
	}
	for _, s := range bad {
		if err := validateClientSpec(s); err == nil {
			t.Errorf("bad spec %q: expected error", s)
		}
	}
}

func TestValidateExportOptions(t *testing.T) {
	good := []string{"rw", "rw,sync,root_squash", "rw,sec=krb5p,fsid=0", "rw,anonuid=65534,anongid=65534", "ro,subtree_check"}
	bad := []string{"", "rw with space", "rw;rm", "rw,`x`", "rw,a=$x", strings.Repeat("a", 257), "rw,opt!"}
	for _, o := range good {
		if err := validateExportOptions(o); err != nil {
			t.Errorf("good opts %q: %v", o, err)
		}
	}
	for _, o := range bad {
		if err := validateExportOptions(o); err == nil {
			t.Errorf("bad opts %q: expected error", o)
		}
	}
}

// ---------- rendering ----------

func TestRenderExportFile(t *testing.T) {
	got := string(renderExportFile(sampleExport()))
	want := "/tank/share1 10.0.0.0/24(rw,sync,root_squash) *(ro)\n"
	if got != want {
		t.Errorf("render mismatch\n got=%q\nwant=%q", got, want)
	}
}

// ---------- CreateExport ----------

func TestManager_CreateExport_OK(t *testing.T) {
	fw := newCaptureFileWriter()
	r := &captureRunner{}
	m := newManager(fw, r)
	if err := m.CreateExport(context.Background(), sampleExport()); err != nil {
		t.Fatal(err)
	}
	wantPath := "/etc/exports.d/nova-nas-share1.exports"
	if len(fw.writes) != 1 || fw.writes[0].Path != wantPath {
		t.Fatalf("writes=%+v", fw.writes)
	}
	if fw.writes[0].Perm != 0o644 {
		t.Errorf("perm=%o", fw.writes[0].Perm)
	}
	wantBody := "/tank/share1 10.0.0.0/24(rw,sync,root_squash) *(ro)\n"
	if string(fw.writes[0].Data) != wantBody {
		t.Errorf("body=%q want=%q", fw.writes[0].Data, wantBody)
	}
	if len(r.calls) != 1 || !eq(r.calls[0], []string{"-ra"}) {
		t.Errorf("runner calls=%v", r.calls)
	}
}

func TestManager_CreateExport_Duplicate(t *testing.T) {
	fw := newCaptureFileWriter()
	fw.files["/etc/exports.d/nova-nas-share1.exports"] = []byte("preexisting\n")
	r := &captureRunner{}
	m := newManager(fw, r)
	err := m.CreateExport(context.Background(), sampleExport())
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want duplicate error, got %v", err)
	}
	if len(fw.writes) != 0 {
		t.Errorf("should not write on duplicate")
	}
	if len(r.calls) != 0 {
		t.Errorf("should not reload on duplicate")
	}
}

func TestManager_CreateExport_BadName(t *testing.T) {
	m := newManager(newCaptureFileWriter(), &captureRunner{})
	e := sampleExport()
	e.Name = "-bad"
	if err := m.CreateExport(context.Background(), e); err == nil {
		t.Fatal("expected error")
	}
}

func TestManager_CreateExport_NoClients(t *testing.T) {
	m := newManager(newCaptureFileWriter(), &captureRunner{})
	e := sampleExport()
	e.Clients = nil
	if err := m.CreateExport(context.Background(), e); err == nil {
		t.Fatal("expected error")
	}
}

// ---------- UpdateExport ----------

func TestManager_UpdateExport_OK(t *testing.T) {
	fw := newCaptureFileWriter()
	fw.files["/etc/exports.d/nova-nas-share1.exports"] = []byte("old\n")
	r := &captureRunner{}
	m := newManager(fw, r)
	e := sampleExport()
	e.Clients = []ClientRule{{Spec: "10.1.0.0/16", Options: "rw,sync"}}
	if err := m.UpdateExport(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	wantBody := "/tank/share1 10.1.0.0/16(rw,sync)\n"
	if string(fw.writes[0].Data) != wantBody {
		t.Errorf("body=%q want=%q", fw.writes[0].Data, wantBody)
	}
	if len(r.calls) != 1 || !eq(r.calls[0], []string{"-ra"}) {
		t.Errorf("runner calls=%v", r.calls)
	}
}

func TestManager_UpdateExport_Missing(t *testing.T) {
	fw := newCaptureFileWriter()
	r := &captureRunner{}
	m := newManager(fw, r)
	err := m.UpdateExport(context.Background(), sampleExport())
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("should not reload on missing")
	}
}

// ---------- DeleteExport ----------

func TestManager_DeleteExport_OK(t *testing.T) {
	fw := newCaptureFileWriter()
	path := "/etc/exports.d/nova-nas-share1.exports"
	fw.files[path] = []byte("body\n")
	r := &captureRunner{}
	m := newManager(fw, r)
	if err := m.DeleteExport(context.Background(), "share1"); err != nil {
		t.Fatal(err)
	}
	if len(fw.removes) != 1 || fw.removes[0] != path {
		t.Errorf("removes=%v", fw.removes)
	}
	if len(r.calls) != 1 || !eq(r.calls[0], []string{"-ra"}) {
		t.Errorf("runner calls=%v", r.calls)
	}
}

func TestManager_DeleteExport_Missing(t *testing.T) {
	fw := newCaptureFileWriter()
	r := &captureRunner{}
	m := newManager(fw, r)
	if err := m.DeleteExport(context.Background(), "share1"); !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	if len(r.calls) != 0 {
		t.Errorf("should not reload on missing")
	}
}

func TestManager_DeleteExport_BadName(t *testing.T) {
	m := newManager(newCaptureFileWriter(), &captureRunner{})
	if err := m.DeleteExport(context.Background(), "-bad"); err == nil {
		t.Fatal("expected error")
	}
}

// ---------- ListExports / GetExport ----------

func TestManager_ListExports_OK(t *testing.T) {
	fw := newCaptureFileWriter()
	fw.dirEntries = []os.DirEntry{
		stubDirEntry{name: "nova-nas-b.exports"},
		stubDirEntry{name: "nova-nas-a.exports"},
		stubDirEntry{name: "other.exports"},        // wrong prefix
		stubDirEntry{name: "nova-nas-foo.txt"},     // wrong suffix
		stubDirEntry{name: "nova-nas-sub", dir: true},
	}
	fw.files["/etc/exports.d/nova-nas-a.exports"] = []byte("/tank/a 10.0.0.0/24(rw)\n")
	fw.files["/etc/exports.d/nova-nas-b.exports"] = []byte("/tank/b *(ro)\n")
	r := &captureRunner{}
	m := newManager(fw, r)
	got, err := m.ListExports(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Name != "a" || got[1].Name != "b" {
		t.Fatalf("got=%+v", got)
	}
	if got[0].Path != "/tank/a" || got[0].Clients[0].Spec != "10.0.0.0/24" {
		t.Errorf("a parsed wrong: %+v", got[0])
	}
}

func TestManager_ListExports_ReadDirError(t *testing.T) {
	fw := newCaptureFileWriter()
	fw.readDirErr = errors.New("boom")
	m := newManager(fw, &captureRunner{})
	if _, err := m.ListExports(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

func TestManager_GetExport_OK(t *testing.T) {
	fw := newCaptureFileWriter()
	fw.files["/etc/exports.d/nova-nas-share1.exports"] = []byte(
		"# managed by nova-nas\n/tank/share1 10.0.0.0/24(rw,sync) *(ro)\n")
	m := newManager(fw, &captureRunner{})
	got, err := m.GetExport(context.Background(), "share1")
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "share1" || got.Path != "/tank/share1" || len(got.Clients) != 2 {
		t.Errorf("got=%+v", got)
	}
}

func TestManager_GetExport_Missing(t *testing.T) {
	fw := newCaptureFileWriter()
	m := newManager(fw, &captureRunner{})
	_, err := m.GetExport(context.Background(), "missing")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestManager_GetExport_BadName(t *testing.T) {
	m := newManager(newCaptureFileWriter(), &captureRunner{})
	if _, err := m.GetExport(context.Background(), "-bad"); err == nil {
		t.Fatal("expected error")
	}
}

// ---------- Reload ----------

func TestManager_Reload_OK(t *testing.T) {
	r := &captureRunner{}
	m := newManager(newCaptureFileWriter(), r)
	if err := m.Reload(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(r.calls) != 1 || !eq(r.calls[0], []string{"-ra"}) {
		t.Errorf("runner calls=%v", r.calls)
	}
}

func TestManager_Reload_Error(t *testing.T) {
	r := &captureRunner{err: errors.New("boom")}
	m := newManager(newCaptureFileWriter(), r)
	if err := m.Reload(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

// ---------- ListActive ----------

func TestManager_ListActive_OK(t *testing.T) {
	r := &captureRunner{out: []byte(
		"/tank/a   10.0.0.0/24(rw,sync,root_squash,no_subtree_check)\n" +
			"/tank/b\n" +
			"\t*(ro,sync,root_squash,no_subtree_check)\n",
	)}
	m := newManager(newCaptureFileWriter(), r)
	got, err := m.ListActive(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("got=%+v", got)
	}
	if got[0].Path != "/tank/a" || got[0].Client != "10.0.0.0/24" || !strings.Contains(got[0].Options, "rw") {
		t.Errorf("first row wrong: %+v", got[0])
	}
	if got[1].Path != "/tank/b" || got[1].Client != "*" || !strings.Contains(got[1].Options, "ro") {
		t.Errorf("second row wrong: %+v", got[1])
	}
	if len(r.calls) != 1 || !eq(r.calls[0], []string{"-v"}) {
		t.Errorf("runner calls=%v", r.calls)
	}
}

func TestManager_ListActive_Error(t *testing.T) {
	r := &captureRunner{err: errors.New("boom")}
	m := newManager(newCaptureFileWriter(), r)
	if _, err := m.ListActive(context.Background()); err == nil {
		t.Fatal("expected error")
	}
}

// ---------- parsers (table-driven) ----------

func TestParseExportFile(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
		path    string
		nClient int
	}{
		{"basic", "/tank/x 10.0.0.0/24(rw)\n", false, "/tank/x", 1},
		{"with comments", "# header\n\n/tank/y *(ro) 10.0.0.5(rw,sync)\n", false, "/tank/y", 2},
		{"empty", "", true, "", 0},
		{"only comments", "# nothing\n", true, "", 0},
		{"missing clients", "/tank/x\n", true, "", 0},
		{"malformed token", "/tank/x notparen\n", true, "", 0},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseExportFile([]byte(c.input))
			if (err != nil) != c.wantErr {
				t.Fatalf("err=%v wantErr=%v", err, c.wantErr)
			}
			if c.wantErr {
				return
			}
			if got.Path != c.path || len(got.Clients) != c.nClient {
				t.Errorf("got=%+v", got)
			}
		})
	}
}

func TestParseExportfsV(t *testing.T) {
	cases := []struct {
		name  string
		input string
		want  []ActiveExport
	}{
		{
			name:  "single line",
			input: "/tank/a 10.0.0.0/24(rw,sync)\n",
			want:  []ActiveExport{{Path: "/tank/a", Client: "10.0.0.0/24", Options: "rw,sync"}},
		},
		{
			name:  "wrapped",
			input: "/tank/b\n\t*(ro)\n",
			want:  []ActiveExport{{Path: "/tank/b", Client: "*", Options: "ro"}},
		},
		{
			name:  "two same line",
			input: "/tank/c 10.0.0.5(rw) host.example.com(ro)\n",
			want: []ActiveExport{
				{Path: "/tank/c", Client: "10.0.0.5", Options: "rw"},
				{Path: "/tank/c", Client: "host.example.com", Options: "ro"},
			},
		},
		{
			name:  "blank lines tolerated",
			input: "\n/tank/d 10.0.0.0/24(rw)\n\n",
			want:  []ActiveExport{{Path: "/tank/d", Client: "10.0.0.0/24", Options: "rw"}},
		},
		{
			name:  "garbage tokens skipped",
			input: "/tank/e nope 10.0.0.0/24(rw)\n",
			want:  []ActiveExport{{Path: "/tank/e", Client: "10.0.0.0/24", Options: "rw"}},
		},
		{
			name:  "empty",
			input: "",
			want:  nil,
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got, err := parseExportfsV([]byte(c.input))
			if err != nil {
				t.Fatal(err)
			}
			if !eq(got, c.want) {
				t.Errorf("got=%+v want=%+v", got, c.want)
			}
		})
	}
}

// Sanity: defaults are populated when fields left zero.
func TestManagerDefaults(t *testing.T) {
	m := &Manager{}
	if m.bin() != "/usr/sbin/exportfs" || m.dir() != "/etc/exports.d" || m.prefix() != "nova-nas-" {
		t.Errorf("defaults wrong: bin=%s dir=%s prefix=%s", m.bin(), m.dir(), m.prefix())
	}
	if m.fw() == nil {
		t.Errorf("fw nil")
	}
}

// ---------- RequireKerberos ----------

func TestApplyKerberosDefaultDisabled(t *testing.T) {
	m := &Manager{} // RequireKerberos: false
	out := m.applyKerberosDefault(sampleExport())
	if out.Clients[0].Options != "rw,sync,root_squash" {
		t.Errorf("options should be unchanged, got %q", out.Clients[0].Options)
	}
}

func TestApplyKerberosDefaultPrependsKrb5p(t *testing.T) {
	m := &Manager{RequireKerberos: true}
	out := m.applyKerberosDefault(sampleExport())
	for i, c := range out.Clients {
		if !strings.HasPrefix(c.Options, "sec=krb5p,") {
			t.Errorf("client[%d] options = %q, want sec=krb5p prefix", i, c.Options)
		}
	}
}

func TestApplyKerberosDefaultRespectsExplicitSec(t *testing.T) {
	m := &Manager{RequireKerberos: true}
	in := Export{
		Name: "share1",
		Path: "/tank/share1",
		Clients: []ClientRule{
			{Spec: "10.0.0.0/24", Options: "rw,sec=krb5i,sync"},
			{Spec: "*", Options: "ro,sec=sys"},
			{Spec: "10.1.0.0/16", Options: "rw"},
		},
	}
	out := m.applyKerberosDefault(in)
	if out.Clients[0].Options != "rw,sec=krb5i,sync" {
		t.Errorf("explicit sec=krb5i must win, got %q", out.Clients[0].Options)
	}
	if out.Clients[1].Options != "ro,sec=sys" {
		t.Errorf("explicit sec=sys must win, got %q", out.Clients[1].Options)
	}
	if out.Clients[2].Options != "sec=krb5p,rw" {
		t.Errorf("missing sec must get krb5p prefix, got %q", out.Clients[2].Options)
	}
}

func TestApplyKerberosDefaultDoesNotMutateInput(t *testing.T) {
	m := &Manager{RequireKerberos: true}
	in := sampleExport()
	orig := in.Clients[0].Options
	_ = m.applyKerberosDefault(in)
	if in.Clients[0].Options != orig {
		t.Errorf("input mutated: %q != %q", in.Clients[0].Options, orig)
	}
}

func TestManager_CreateExport_RequireKerberos(t *testing.T) {
	fw := newCaptureFileWriter()
	r := &captureRunner{}
	m := newManager(fw, r)
	m.RequireKerberos = true
	if err := m.CreateExport(context.Background(), sampleExport()); err != nil {
		t.Fatal(err)
	}
	body := string(fw.writes[0].Data)
	want := "/tank/share1 10.0.0.0/24(sec=krb5p,rw,sync,root_squash) *(sec=krb5p,ro)\n"
	if body != want {
		t.Errorf("body=%q\nwant=%q", body, want)
	}
}

func TestManager_UpdateExport_RequireKerberos(t *testing.T) {
	fw := newCaptureFileWriter()
	fw.files["/etc/exports.d/nova-nas-share1.exports"] = []byte("old\n")
	r := &captureRunner{}
	m := newManager(fw, r)
	m.RequireKerberos = true
	e := sampleExport()
	e.Clients = []ClientRule{{Spec: "10.1.0.0/16", Options: "rw,sync"}}
	if err := m.UpdateExport(context.Background(), e); err != nil {
		t.Fatal(err)
	}
	body := string(fw.writes[0].Data)
	want := "/tank/share1 10.1.0.0/16(sec=krb5p,rw,sync)\n"
	if body != want {
		t.Errorf("body=%q\nwant=%q", body, want)
	}
}

func TestHasSecOption(t *testing.T) {
	cases := []struct {
		opts string
		want bool
	}{
		{"rw", false},
		{"rw,sync", false},
		{"sec=krb5p", true},
		{"rw,sec=krb5p,sync", true},
		{"rw,SEC=krb5i", true},
		{"rw,fsid=0", false},
		{"", false},
		{"sec=", true}, // explicit empty value is still an explicit override
	}
	for _, c := range cases {
		if got := hasSecOption(c.opts); got != c.want {
			t.Errorf("hasSecOption(%q)=%v want %v", c.opts, got, c.want)
		}
	}
}

// Compile-time guard so tests don't drift past timeouts during local runs.
var _ = time.Second
