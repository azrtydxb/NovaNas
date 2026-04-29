package samba

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// ---------- validation ----------

func TestValidateGlobals(t *testing.T) {
	good := GlobalsOpts{
		Workgroup:    "WORKGROUP",
		ServerString: "NovaNAS",
		ACLProfile:   "nfsv4",
		SecurityMode: "user",
	}
	if err := validateGlobals(good); err != nil {
		t.Fatalf("expected good opts to pass: %v", err)
	}

	cases := []struct {
		name string
		mut  func(*GlobalsOpts)
	}{
		{"bad-acl-profile", func(o *GlobalsOpts) { o.ACLProfile = "junk" }},
		{"bad-security-mode", func(o *GlobalsOpts) { o.SecurityMode = "junk" }},
		{"ads-without-realm", func(o *GlobalsOpts) { o.SecurityMode = "ads"; o.Realm = "" }},
		{"custom-line-no-eq", func(o *GlobalsOpts) { o.CustomLines = []string{"no equals here"} }},
		{"custom-line-shell-meta-semi", func(o *GlobalsOpts) { o.CustomLines = []string{"foo = bar; rm -rf /"} }},
		{"custom-line-shell-meta-pipe", func(o *GlobalsOpts) { o.CustomLines = []string{"foo = bar | cat"} }},
		{"custom-line-shell-meta-backtick", func(o *GlobalsOpts) { o.CustomLines = []string{"foo = `id`"} }},
		{"custom-line-shell-meta-dollar", func(o *GlobalsOpts) { o.CustomLines = []string{"foo = $(id)"} }},
		{"custom-line-newline", func(o *GlobalsOpts) { o.CustomLines = []string{"foo = bar\nrogue = yes"} }},
		{"workgroup-empty", func(o *GlobalsOpts) { o.Workgroup = "" }},
		{"workgroup-with-space", func(o *GlobalsOpts) { o.Workgroup = "BAD WG" }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			o := good
			tc.mut(&o)
			if err := validateGlobals(o); err == nil {
				t.Errorf("expected validation error for %s", tc.name)
			}
		})
	}
}

// ---------- render / parse round-trip ----------

func TestGlobalsRoundTripNFSv4(t *testing.T) {
	want := GlobalsOpts{
		Workgroup:     "HOMELAB",
		ServerString:  "NovaNAS prod",
		ACLProfile:    "nfsv4",
		SecurityMode:  "user",
		EnableNetBIOS: false,
	}
	want = applyGlobalsDefaults(want)
	data := renderGlobalsConf(want)
	got, err := parseGlobalsConf(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// CustomLines and Realm aren't recovered through the round-trip
	// for this case (no realm, no custom lines), so compare directly.
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("round-trip mismatch:\nwant: %+v\ngot:  %+v", want, *got)
	}
	// Sanity checks on the rendered text.
	s := string(data)
	for _, must := range []string{
		"[global]",
		"workgroup = HOMELAB",
		"server string = NovaNAS prod",
		"server min protocol = SMB2",
		"server max protocol = SMB3",
		"client min protocol = SMB2",
		"vfs objects = zfsacl",
		"oplocks = no",
		"idmap config * : range = 100000-200000",
		"disable netbios = yes",
	} {
		if !strings.Contains(s, must) {
			t.Errorf("rendered nfsv4 missing %q\n%s", must, s)
		}
	}
}

func TestGlobalsRoundTripPosix(t *testing.T) {
	want := GlobalsOpts{
		Workgroup:     "WG",
		ServerString:  "posix-box",
		ACLProfile:    "posix",
		SecurityMode:  "user",
		EnableNetBIOS: true, // omit "disable netbios = yes"
	}
	want = applyGlobalsDefaults(want)
	data := renderGlobalsConf(want)
	got, err := parseGlobalsConf(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("round-trip mismatch:\nwant: %+v\ngot:  %+v", want, *got)
	}
	s := string(data)
	for _, must := range []string{
		"[global]",
		"vfs objects = acl_xattr",
		"acl_xattr:ignore system acls = yes",
	} {
		if !strings.Contains(s, must) {
			t.Errorf("rendered posix missing %q\n%s", must, s)
		}
	}
	if strings.Contains(s, "disable netbios") {
		t.Errorf("posix with EnableNetBIOS=true should not emit 'disable netbios': %s", s)
	}
	if strings.Contains(s, "zfsacl") {
		t.Errorf("posix profile should not emit zfsacl: %s", s)
	}
	if strings.Contains(s, "client min protocol") {
		t.Errorf("posix profile spec doesn't mandate client min protocol: %s", s)
	}
}

func TestGlobalsRoundTripADS(t *testing.T) {
	want := applyGlobalsDefaults(GlobalsOpts{
		SecurityMode: "ads",
		Realm:        "EXAMPLE.COM",
	})
	if err := validateGlobals(want); err != nil {
		t.Fatalf("validate: %v", err)
	}
	data := renderGlobalsConf(want)
	got, err := parseGlobalsConf(data)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if got.SecurityMode != "ads" || got.Realm != "EXAMPLE.COM" {
		t.Errorf("ads round-trip lost values: %+v", got)
	}
}

// ---------- SetGlobals + rollback ----------

func TestSetGlobalsWritesAndReloads(t *testing.T) {
	cr := &captureRunner{}
	fw := newCaptureFileWriter()
	m := newManagerWith(cr, nil, fw)

	c, cancel := ctx()
	defer cancel()
	if err := m.SetGlobals(c, GlobalsOpts{}); err != nil {
		t.Fatalf("SetGlobals: %v", err)
	}
	path := "/etc/samba/smb.conf.d/00-nova-globals.conf"
	if _, ok := fw.files[path]; !ok {
		t.Fatalf("globals file not written at %s; have: %v", path, fw.files)
	}
	// Verify both testparm and smbcontrol ran.
	var sawTestparm, sawSmbcontrol bool
	for _, call := range cr.calls {
		if strings.Contains(call[0], "testparm") {
			sawTestparm = true
		}
		if strings.Contains(call[0], "smbcontrol") {
			sawSmbcontrol = true
		}
	}
	if !sawTestparm || !sawSmbcontrol {
		t.Errorf("expected testparm and smbcontrol calls, got: %+v", cr.calls)
	}
}

func TestSetGlobalsRollbackRestoresPrevious(t *testing.T) {
	fw := newCaptureFileWriter()
	path := "/etc/samba/smb.conf.d/00-nova-globals.conf"
	prev := []byte("[global]\n   workgroup = OLD\n   vfs objects = zfsacl\n")
	fw.files[path] = append([]byte(nil), prev...)

	cr := &captureRunner{
		errFor: map[string]error{
			"/usr/bin/testparm": errors.New("boom"),
		},
	}
	m := newManagerWith(cr, nil, fw)

	c, cancel := ctx()
	defer cancel()
	err := m.SetGlobals(c, GlobalsOpts{Workgroup: "NEW"})
	if err == nil {
		t.Fatal("expected error from failing testparm")
	}
	got, ok := fw.files[path]
	if !ok {
		t.Fatal("file removed; expected restore to previous content")
	}
	if string(got) != string(prev) {
		t.Errorf("file not restored:\nwant: %q\ngot:  %q", prev, got)
	}
}

func TestSetGlobalsRollbackRemovesWhenNoPrevious(t *testing.T) {
	fw := newCaptureFileWriter()
	cr := &captureRunner{
		errFor: map[string]error{
			"/usr/bin/testparm": errors.New("boom"),
		},
	}
	m := newManagerWith(cr, nil, fw)

	c, cancel := ctx()
	defer cancel()
	err := m.SetGlobals(c, GlobalsOpts{})
	if err == nil {
		t.Fatal("expected error from failing testparm")
	}
	path := "/etc/samba/smb.conf.d/00-nova-globals.conf"
	if _, ok := fw.files[path]; ok {
		t.Errorf("expected file to be removed after failed write with no previous, but it remains")
	}
}

func TestSetGlobalsRejectsBadInput(t *testing.T) {
	fw := newCaptureFileWriter()
	cr := &captureRunner{}
	m := newManagerWith(cr, nil, fw)

	c, cancel := ctx()
	defer cancel()
	err := m.SetGlobals(c, GlobalsOpts{ACLProfile: "junk"})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if len(fw.writes) != 0 {
		t.Errorf("validation failure should not write any file, got: %v", fw.writes)
	}
	if len(cr.calls) != 0 {
		t.Errorf("validation failure should not run any command, got: %v", cr.calls)
	}
}

// ---------- GetGlobals ----------

func TestGetGlobalsMissingReturnsZero(t *testing.T) {
	fw := newCaptureFileWriter()
	m := newManagerWith(nil, nil, fw)
	c, cancel := ctx()
	defer cancel()
	got, err := m.GetGlobals(c)
	if err != nil {
		t.Fatalf("expected nil error for missing globals, got: %v", err)
	}
	if got == nil {
		t.Fatal("expected non-nil zero-value GlobalsOpts")
	}
	if !reflect.DeepEqual(*got, GlobalsOpts{}) {
		t.Errorf("expected zero-value, got: %+v", *got)
	}
}

func TestGetGlobalsAfterSet(t *testing.T) {
	cr := &captureRunner{}
	fw := newCaptureFileWriter()
	m := newManagerWith(cr, nil, fw)

	c, cancel := ctx()
	defer cancel()
	want := GlobalsOpts{
		Workgroup:     "MYWG",
		ServerString:  "Nova Test",
		ACLProfile:    "nfsv4",
		SecurityMode:  "user",
		EnableNetBIOS: false,
	}
	if err := m.SetGlobals(c, want); err != nil {
		t.Fatalf("SetGlobals: %v", err)
	}
	got, err := m.GetGlobals(c)
	if err != nil {
		t.Fatalf("GetGlobals: %v", err)
	}
	want = applyGlobalsDefaults(want)
	if !reflect.DeepEqual(*got, want) {
		t.Errorf("GetGlobals mismatch:\nwant: %+v\ngot:  %+v", want, *got)
	}
}
