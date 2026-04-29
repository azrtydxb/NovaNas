package samba

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

type captureRunner struct {
	calls   [][]string // each call: [bin, args...]
	out     []byte
	err     error
	errFor  map[string]error // by bin
	outFor  map[string][]byte
}

func (c *captureRunner) run(_ context.Context, bin string, args ...string) ([]byte, error) {
	cp := append([]string{bin}, args...)
	c.calls = append(c.calls, cp)
	if e, ok := c.errFor[bin]; ok {
		return c.outFor[bin], e
	}
	if o, ok := c.outFor[bin]; ok {
		return o, nil
	}
	return c.out, c.err
}

type stdinCall struct {
	Bin   string
	Stdin []byte
	Args  []string
}

type captureStdinRunner struct {
	calls []stdinCall
	out   []byte
	err   error
}

func (c *captureStdinRunner) run(_ context.Context, bin string, stdin []byte, args ...string) ([]byte, error) {
	cp := append([]byte(nil), stdin...)
	ac := append([]string(nil), args...)
	c.calls = append(c.calls, stdinCall{Bin: bin, Stdin: cp, Args: ac})
	return c.out, c.err
}

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

type stubDirEntry struct {
	name string
	dir  bool
}

func (s stubDirEntry) Name() string               { return s.name }
func (s stubDirEntry) IsDir() bool                { return s.dir }
func (s stubDirEntry) Type() os.FileMode          { return 0 }
func (s stubDirEntry) Info() (os.FileInfo, error) { return nil, nil }

func newManagerWith(cr *captureRunner, sr *captureStdinRunner, fw *captureFileWriter) *Manager {
	m := &Manager{
		ConfigDir:  "/etc/samba/smb.conf.d",
		FilePrefix: "nova-nas-",
		FileWriter: fw,
	}
	if cr != nil {
		m.Runner = cr.run
	}
	if sr != nil {
		m.StdinRunner = sr.run
	}
	return m
}

func ctx() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 5*time.Second)
}

// ---------- renderShareConf ----------

func TestRenderShareConf(t *testing.T) {
	tests := []struct {
		name string
		s    Share
		want string
	}{
		{
			name: "minimal",
			s:    Share{Name: "data", Path: "/tank/data"},
			want: "[data]\n   path = /tank/data\n   browseable = no\n   writable = no\n   guest ok = no\n   read only = no\n",
		},
		{
			name: "all-flags-true",
			s: Share{
				Name: "pub", Path: "/tank/pub",
				Browseable: true, Writable: true, GuestOK: true, ReadOnly: true,
			},
			want: "[pub]\n   path = /tank/pub\n   browseable = yes\n   writable = yes\n   guest ok = yes\n   read only = yes\n",
		},
		{
			name: "with-comment",
			s:    Share{Name: "x", Path: "/x", Comment: "a comment"},
			want: "[x]\n   path = /x\n   comment = a comment\n   browseable = no\n   writable = no\n   guest ok = no\n   read only = no\n",
		},
		{
			name: "valid-users",
			s:    Share{Name: "x", Path: "/x", ValidUsers: []string{"alice", "bob"}},
			want: "[x]\n   path = /x\n   browseable = no\n   writable = no\n   guest ok = no\n   read only = no\n   valid users = alice, bob\n",
		},
		{
			name: "write-list",
			s:    Share{Name: "x", Path: "/x", WriteList: []string{"alice"}},
			want: "[x]\n   path = /x\n   browseable = no\n   writable = no\n   guest ok = no\n   read only = no\n   write list = alice\n",
		},
		{
			name: "admin-users",
			s:    Share{Name: "x", Path: "/x", AdminUsers: []string{"root"}},
			want: "[x]\n   path = /x\n   browseable = no\n   writable = no\n   guest ok = no\n   read only = no\n   admin users = root\n",
		},
		{
			name: "masks",
			s:    Share{Name: "x", Path: "/x", CreateMask: "0644", DirectoryMask: "0755"},
			want: "[x]\n   path = /x\n   browseable = no\n   writable = no\n   guest ok = no\n   read only = no\n   create mask = 0644\n   directory mask = 0755\n",
		},
		{
			name: "veto",
			s:    Share{Name: "x", Path: "/x", Veto: []string{".DS_Store", "Thumbs.db"}},
			want: "[x]\n   path = /x\n   browseable = no\n   writable = no\n   guest ok = no\n   read only = no\n   veto files = /.DS_Store/Thumbs.db/\n",
		},
		{
			name: "everything",
			s: Share{
				Name: "all", Path: "/tank/all", Comment: "kitchen sink",
				Browseable: true, Writable: true, GuestOK: false, ReadOnly: false,
				ValidUsers: []string{"alice", "bob"}, WriteList: []string{"bob"},
				AdminUsers: []string{"root"},
				CreateMask: "0660", DirectoryMask: "0770",
				Veto: []string{"._*"},
			},
			want: "[all]\n   path = /tank/all\n   comment = kitchen sink\n   browseable = yes\n   writable = yes\n   guest ok = no\n   read only = no\n   valid users = alice, bob\n   write list = bob\n   admin users = root\n   create mask = 0660\n   directory mask = 0770\n   veto files = /._*/\n",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := string(renderShareConf(tc.s))
			if got != tc.want {
				t.Errorf("renderShareConf mismatch:\nwant:\n%q\ngot:\n%q", tc.want, got)
			}
		})
	}
}

// ---------- parseShareConf round-trip ----------

func TestParseShareConfRoundTrip(t *testing.T) {
	cases := []Share{
		{Name: "data", Path: "/tank/data"},
		{
			Name: "all", Path: "/tank/all", Comment: "kitchen sink",
			Browseable: true, Writable: true,
			ValidUsers: []string{"alice", "bob"},
			WriteList:  []string{"bob"},
			AdminUsers: []string{"root"},
			CreateMask: "0660", DirectoryMask: "0770",
			Veto: []string{"._*", ".DS_Store"},
		},
		{
			Name: "guest", Path: "/tank/guest", GuestOK: true, ReadOnly: true,
		},
	}
	for _, want := range cases {
		t.Run(want.Name, func(t *testing.T) {
			data := renderShareConf(want)
			got, err := parseShareConf(data)
			if err != nil {
				t.Fatalf("parse: %v", err)
			}
			if !reflect.DeepEqual(*got, want) {
				t.Errorf("round-trip mismatch:\nwant: %+v\ngot:  %+v", want, *got)
			}
		})
	}
}

func TestParseShareConfErrors(t *testing.T) {
	if _, err := parseShareConf([]byte("path = /x\n")); err == nil {
		t.Error("expected error for missing section header")
	}
	if _, err := parseShareConf([]byte("# only comments\n\n")); err == nil {
		t.Error("expected error for empty content")
	}
}

func TestParseShareConfIgnoresExtras(t *testing.T) {
	body := "# leading comment\n[x]\n   path = /a\n   unknown = yes\n   browseable = yes\n; comment\n"
	sh, err := parseShareConf([]byte(body))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if sh.Path != "/a" || !sh.Browseable {
		t.Errorf("bad parse: %+v", sh)
	}
}

// ---------- parsePdbeditL ----------

func TestParsePdbeditL(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []string
	}{
		{"empty", "", nil},
		{"one", "alice:1001:Alice\n", []string{"alice"}},
		{"many", "alice:1001:Alice\nbob:1002:Bob\nroot:0:root\n", []string{"alice", "bob", "root"}},
		{"trailing-blank", "alice:1001:Alice\n\n", []string{"alice"}},
		{"no-colons", "alice\n", []string{"alice"}},
		{"bad-username-skipped", "alice:1001:Alice\n-bad:1002:x\n", []string{"alice"}},
		{"whitespace-trimmed", "  alice:1001:Alice  \n", []string{"alice"}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := parsePdbeditL([]byte(tc.in))
			var names []string
			for _, u := range got {
				names = append(names, u.Username)
			}
			if !reflect.DeepEqual(names, tc.want) {
				t.Errorf("want %v got %v", tc.want, names)
			}
		})
	}
}

// ---------- validation ----------

func TestValidateShareName(t *testing.T) {
	good := []string{"data", "data_1", "data-2", "X", "a"}
	bad := []string{"", "-bad", "has space", "with/slash", "ümlaut", strings.Repeat("a", 65)}
	for _, n := range good {
		if err := validateShareName(n); err != nil {
			t.Errorf("good %q rejected: %v", n, err)
		}
	}
	for _, n := range bad {
		if err := validateShareName(n); err == nil {
			t.Errorf("bad %q accepted", n)
		}
	}
}

func TestValidateSharePath(t *testing.T) {
	good := []string{"/tank/data", "/a", "/a/b/c"}
	bad := []string{"", "relative", "/a/../b", "/a b", "/a;b", "/a$b", "/a\nb"}
	for _, p := range good {
		if err := validateSharePath(p); err != nil {
			t.Errorf("good %q rejected: %v", p, err)
		}
	}
	for _, p := range bad {
		if err := validateSharePath(p); err == nil {
			t.Errorf("bad %q accepted", p)
		}
	}
}

func TestValidateUsername(t *testing.T) {
	good := []string{"alice", "_svc", "user.name", "user-1", "u"}
	bad := []string{"", "1user", "-u", "user name", "us;er", strings.Repeat("a", 33)}
	for _, u := range good {
		if err := validateUsername(u); err != nil {
			t.Errorf("good %q rejected: %v", u, err)
		}
	}
	for _, u := range bad {
		if err := validateUsername(u); err == nil {
			t.Errorf("bad %q accepted", u)
		}
	}
}

// ---------- CreateShare ----------

func TestCreateShareSuccess(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{}
	fw := newCaptureFileWriter()
	m := newManagerWith(cr, nil, fw)
	s := Share{Name: "data", Path: "/tank/data", Browseable: true}
	if err := m.CreateShare(c, s); err != nil {
		t.Fatalf("CreateShare: %v", err)
	}
	path := "/etc/samba/smb.conf.d/nova-nas-data.conf"
	if _, ok := fw.files[path]; !ok {
		t.Fatalf("expected file %s to be written", path)
	}
	// Two calls: testparm -s, smbcontrol smbd reload-config
	if len(cr.calls) != 2 {
		t.Fatalf("expected 2 runner calls, got %d: %v", len(cr.calls), cr.calls)
	}
	if cr.calls[0][0] != "/usr/bin/testparm" || cr.calls[0][1] != "-s" {
		t.Errorf("first call should be testparm -s, got %v", cr.calls[0])
	}
	if cr.calls[1][0] != "/usr/bin/smbcontrol" || cr.calls[1][1] != "smbd" || cr.calls[1][2] != "reload-config" {
		t.Errorf("second call should be smbcontrol smbd reload-config, got %v", cr.calls[1])
	}
}

func TestCreateShareDuplicate(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	fw := newCaptureFileWriter()
	path := "/etc/samba/smb.conf.d/nova-nas-data.conf"
	fw.files[path] = []byte("existing")
	m := newManagerWith(&captureRunner{}, nil, fw)
	err := m.CreateShare(c, Share{Name: "data", Path: "/tank/data"})
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Errorf("expected duplicate error, got %v", err)
	}
}

func TestCreateShareValidationFailure(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(&captureRunner{}, nil, newCaptureFileWriter())
	if err := m.CreateShare(c, Share{Name: "bad name", Path: "/x"}); err == nil {
		t.Error("expected validation error for bad name")
	}
	if err := m.CreateShare(c, Share{Name: "ok", Path: "rel"}); err == nil {
		t.Error("expected validation error for relative path")
	}
}

func TestCreateShareTestparmFailureRollsBack(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{
		errFor: map[string]error{"/usr/bin/testparm": errors.New("bad config")},
	}
	fw := newCaptureFileWriter()
	m := newManagerWith(cr, nil, fw)
	err := m.CreateShare(c, Share{Name: "data", Path: "/tank/data"})
	if err == nil {
		t.Fatal("expected testparm failure")
	}
	if _, ok := fw.files["/etc/samba/smb.conf.d/nova-nas-data.conf"]; ok {
		t.Error("expected file to be rolled back on testparm failure")
	}
	// reload should not have been called
	for _, call := range cr.calls {
		if call[0] == "/usr/bin/smbcontrol" {
			t.Error("smbcontrol should not be called after testparm failure")
		}
	}
}

// ---------- UpdateShare ----------

func TestUpdateShareSuccess(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{}
	fw := newCaptureFileWriter()
	path := "/etc/samba/smb.conf.d/nova-nas-data.conf"
	fw.files[path] = []byte("[data]\n   path = /old\n")
	m := newManagerWith(cr, nil, fw)
	if err := m.UpdateShare(c, Share{Name: "data", Path: "/new"}); err != nil {
		t.Fatalf("UpdateShare: %v", err)
	}
	if !strings.Contains(string(fw.files[path]), "/new") {
		t.Error("expected file to be updated with new path")
	}
}

func TestUpdateShareNotFound(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(&captureRunner{}, nil, newCaptureFileWriter())
	err := m.UpdateShare(c, Share{Name: "missing", Path: "/x"})
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestUpdateShareTestparmFailureRestoresPrev(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{
		errFor: map[string]error{"/usr/bin/testparm": errors.New("bad config")},
	}
	fw := newCaptureFileWriter()
	path := "/etc/samba/smb.conf.d/nova-nas-data.conf"
	prev := []byte("[data]\n   path = /old\n")
	fw.files[path] = append([]byte(nil), prev...)
	m := newManagerWith(cr, nil, fw)
	err := m.UpdateShare(c, Share{Name: "data", Path: "/new"})
	if err == nil {
		t.Fatal("expected testparm failure")
	}
	if string(fw.files[path]) != string(prev) {
		t.Errorf("expected previous content restored, got %q", fw.files[path])
	}
}

// ---------- DeleteShare ----------

func TestDeleteShareSuccess(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{}
	fw := newCaptureFileWriter()
	path := "/etc/samba/smb.conf.d/nova-nas-data.conf"
	fw.files[path] = []byte("x")
	m := newManagerWith(cr, nil, fw)
	if err := m.DeleteShare(c, "data"); err != nil {
		t.Fatalf("DeleteShare: %v", err)
	}
	if _, ok := fw.files[path]; ok {
		t.Error("expected file to be removed")
	}
	if len(cr.calls) != 1 || cr.calls[0][0] != "/usr/bin/smbcontrol" {
		t.Errorf("expected single smbcontrol reload call, got %v", cr.calls)
	}
}

func TestDeleteShareNotFound(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(&captureRunner{}, nil, newCaptureFileWriter())
	if err := m.DeleteShare(c, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestDeleteShareBadName(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(&captureRunner{}, nil, newCaptureFileWriter())
	if err := m.DeleteShare(c, "bad name"); err == nil {
		t.Error("expected validation error")
	}
}

// ---------- ListShares / GetShare ----------

func TestListShares(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	fw := newCaptureFileWriter()
	fw.dirEntries = []os.DirEntry{
		stubDirEntry{name: "nova-nas-alpha.conf"},
		stubDirEntry{name: "nova-nas-beta.conf"},
		stubDirEntry{name: "ignored.conf"},
		stubDirEntry{name: "nova-nas-skipme.txt"},
		stubDirEntry{name: "subdir", dir: true},
	}
	fw.files["/etc/samba/smb.conf.d/nova-nas-alpha.conf"] = renderShareConf(Share{Name: "alpha", Path: "/a"})
	fw.files["/etc/samba/smb.conf.d/nova-nas-beta.conf"] = renderShareConf(Share{Name: "beta", Path: "/b"})
	m := newManagerWith(&captureRunner{}, nil, fw)
	got, err := m.ListShares(c)
	if err != nil {
		t.Fatalf("ListShares: %v", err)
	}
	if len(got) != 2 || got[0].Name != "alpha" || got[1].Name != "beta" {
		t.Errorf("unexpected list: %+v", got)
	}
}

func TestGetShareSuccess(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	fw := newCaptureFileWriter()
	fw.files["/etc/samba/smb.conf.d/nova-nas-x.conf"] = renderShareConf(Share{Name: "x", Path: "/x", Browseable: true})
	m := newManagerWith(&captureRunner{}, nil, fw)
	got, err := m.GetShare(c, "x")
	if err != nil {
		t.Fatalf("GetShare: %v", err)
	}
	if got.Name != "x" || got.Path != "/x" || !got.Browseable {
		t.Errorf("unexpected: %+v", got)
	}
}

func TestGetShareNotFound(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(&captureRunner{}, nil, newCaptureFileWriter())
	if _, err := m.GetShare(c, "missing"); !errors.Is(err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestGetShareBadName(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(&captureRunner{}, nil, newCaptureFileWriter())
	if _, err := m.GetShare(c, "bad name"); err == nil {
		t.Error("expected validation error")
	}
}

// ---------- Reload ----------

func TestReload(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{}
	m := newManagerWith(cr, nil, newCaptureFileWriter())
	if err := m.Reload(c); err != nil {
		t.Fatalf("Reload: %v", err)
	}
	if len(cr.calls) != 1 || cr.calls[0][0] != "/usr/bin/smbcontrol" || cr.calls[0][1] != "smbd" || cr.calls[0][2] != "reload-config" {
		t.Errorf("unexpected call: %v", cr.calls)
	}
}

func TestReloadFailure(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{err: errors.New("boom")}
	m := newManagerWith(cr, nil, newCaptureFileWriter())
	if err := m.Reload(c); err == nil {
		t.Error("expected error")
	}
}

// ---------- AddUser / DeleteUser / SetUserPassword / ListUsers ----------

func TestAddUserSuccess(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	sr := &captureStdinRunner{}
	m := newManagerWith(nil, sr, newCaptureFileWriter())
	if err := m.AddUser(c, "alice", "secret"); err != nil {
		t.Fatalf("AddUser: %v", err)
	}
	if len(sr.calls) != 1 {
		t.Fatalf("expected 1 stdin call, got %d", len(sr.calls))
	}
	want := "secret\nsecret\n"
	if string(sr.calls[0].Stdin) != want {
		t.Errorf("stdin: want %q got %q", want, sr.calls[0].Stdin)
	}
	wantArgs := []string{"-a", "-s", "alice"}
	if !reflect.DeepEqual(sr.calls[0].Args, wantArgs) {
		t.Errorf("args: want %v got %v", wantArgs, sr.calls[0].Args)
	}
}

func TestAddUserBadUsername(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(nil, &captureStdinRunner{}, newCaptureFileWriter())
	if err := m.AddUser(c, "-bad", "x"); err == nil {
		t.Error("expected validation error")
	}
}

func TestAddUserEmptyPassword(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(nil, &captureStdinRunner{}, newCaptureFileWriter())
	if err := m.AddUser(c, "alice", ""); err == nil {
		t.Error("expected error for empty password")
	}
}

func TestDeleteUserSuccess(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{}
	m := newManagerWith(cr, nil, newCaptureFileWriter())
	if err := m.DeleteUser(c, "alice"); err != nil {
		t.Fatalf("DeleteUser: %v", err)
	}
	if len(cr.calls) != 1 || cr.calls[0][0] != "/usr/bin/smbpasswd" || cr.calls[0][1] != "-x" || cr.calls[0][2] != "alice" {
		t.Errorf("unexpected call: %v", cr.calls)
	}
}

func TestDeleteUserBadName(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(&captureRunner{}, nil, newCaptureFileWriter())
	if err := m.DeleteUser(c, "bad name"); err == nil {
		t.Error("expected validation error")
	}
}

func TestSetUserPasswordSuccess(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	sr := &captureStdinRunner{}
	m := newManagerWith(nil, sr, newCaptureFileWriter())
	if err := m.SetUserPassword(c, "alice", "newpass"); err != nil {
		t.Fatalf("SetUserPassword: %v", err)
	}
	if len(sr.calls) != 1 {
		t.Fatalf("expected 1 stdin call, got %d", len(sr.calls))
	}
	want := "newpass\nnewpass\n"
	if string(sr.calls[0].Stdin) != want {
		t.Errorf("stdin: want %q got %q", want, sr.calls[0].Stdin)
	}
	wantArgs := []string{"-s", "alice"}
	if !reflect.DeepEqual(sr.calls[0].Args, wantArgs) {
		t.Errorf("args: want %v got %v", wantArgs, sr.calls[0].Args)
	}
}

func TestSetUserPasswordBadInput(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	m := newManagerWith(nil, &captureStdinRunner{}, newCaptureFileWriter())
	if err := m.SetUserPassword(c, "-bad", "x"); err == nil {
		t.Error("expected username validation error")
	}
	if err := m.SetUserPassword(c, "alice", ""); err == nil {
		t.Error("expected empty-password error")
	}
}

func TestListUsersSuccess(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{
		outFor: map[string][]byte{
			"/usr/bin/pdbedit": []byte("alice:1001:Alice\nbob:1002:Bob\n"),
		},
	}
	m := newManagerWith(cr, nil, newCaptureFileWriter())
	users, err := m.ListUsers(c)
	if err != nil {
		t.Fatalf("ListUsers: %v", err)
	}
	if len(users) != 2 || users[0].Username != "alice" || users[1].Username != "bob" {
		t.Errorf("unexpected: %+v", users)
	}
}

func TestListUsersFailure(t *testing.T) {
	c, cancel := ctx()
	defer cancel()
	cr := &captureRunner{err: errors.New("boom")}
	m := newManagerWith(cr, nil, newCaptureFileWriter())
	if _, err := m.ListUsers(c); err == nil {
		t.Error("expected error")
	}
}
