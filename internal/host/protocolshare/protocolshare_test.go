package protocolshare

import (
	"context"
	"errors"
	"os"
	"reflect"
	"testing"

	"github.com/novanas/nova-nas/internal/host/nfs"
	"github.com/novanas/nova-nas/internal/host/samba"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

// ---------- stubs ----------

// recorder is a shared call log so tests can assert global ordering
// across all three stubs.
type recorder struct{ calls []string }

func (r *recorder) record(s string) { r.calls = append(r.calls, s) }

type stubDS struct {
	rec      *recorder
	creates  []dataset.CreateSpec
	setProps []struct {
		name  string
		props map[string]string
	}
	destroys []struct {
		name      string
		recursive bool
	}
	gets    map[string]*dataset.Detail
	getErr  map[string]error
	setACLs []struct {
		path string
		aces []ACE
	}
	getACL    []ACE
	getACLErr error

	createErr  error
	setACLErr  error
	destroyErr error
}

func (s *stubDS) Create(_ context.Context, spec dataset.CreateSpec) error {
	s.rec.record("ds.Create")
	if s.createErr != nil {
		return s.createErr
	}
	s.creates = append(s.creates, spec)
	return nil
}
func (s *stubDS) SetProps(_ context.Context, name string, props map[string]string) error {
	s.rec.record("ds.SetProps")
	s.setProps = append(s.setProps, struct {
		name  string
		props map[string]string
	}{name, props})
	return nil
}
func (s *stubDS) Destroy(_ context.Context, name string, recursive bool) error {
	s.rec.record("ds.Destroy")
	s.destroys = append(s.destroys, struct {
		name      string
		recursive bool
	}{name, recursive})
	return s.destroyErr
}
func (s *stubDS) Get(_ context.Context, name string) (*dataset.Detail, error) {
	s.rec.record("ds.Get")
	if e, ok := s.getErr[name]; ok {
		return nil, e
	}
	if d, ok := s.gets[name]; ok {
		return d, nil
	}
	return nil, dataset.ErrNotFound
}
func (s *stubDS) SetACL(_ context.Context, path string, aces []ACE) error {
	s.rec.record("ds.SetACL")
	if s.setACLErr != nil {
		return s.setACLErr
	}
	s.setACLs = append(s.setACLs, struct {
		path string
		aces []ACE
	}{path, aces})
	return nil
}
func (s *stubDS) GetACL(_ context.Context, _ string) ([]ACE, error) {
	s.rec.record("ds.GetACL")
	return s.getACL, s.getACLErr
}

type stubNFS struct {
	rec       *recorder
	creates   []nfs.Export
	updates   []nfs.Export
	deletes   []string
	createErr error
	updateErr error
	deleteErr error
}

func (s *stubNFS) CreateExport(_ context.Context, e nfs.Export) error {
	s.rec.record("nfs.CreateExport")
	if s.createErr != nil {
		return s.createErr
	}
	s.creates = append(s.creates, e)
	return nil
}
func (s *stubNFS) UpdateExport(_ context.Context, e nfs.Export) error {
	s.rec.record("nfs.UpdateExport")
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updates = append(s.updates, e)
	return nil
}
func (s *stubNFS) DeleteExport(_ context.Context, name string) error {
	s.rec.record("nfs.DeleteExport")
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deletes = append(s.deletes, name)
	return nil
}

type stubSamba struct {
	rec        *recorder
	creates    []samba.Share
	updates    []samba.Share
	deletes    []string
	globals    []GlobalsOpts
	createErr  error
	updateErr  error
	deleteErr  error
	globalsErr error
}

func (s *stubSamba) CreateShare(_ context.Context, sh samba.Share) error {
	s.rec.record("smb.CreateShare")
	if s.createErr != nil {
		return s.createErr
	}
	s.creates = append(s.creates, sh)
	return nil
}
func (s *stubSamba) UpdateShare(_ context.Context, sh samba.Share) error {
	s.rec.record("smb.UpdateShare")
	if s.updateErr != nil {
		return s.updateErr
	}
	s.updates = append(s.updates, sh)
	return nil
}
func (s *stubSamba) DeleteShare(_ context.Context, name string) error {
	s.rec.record("smb.DeleteShare")
	if s.deleteErr != nil {
		return s.deleteErr
	}
	s.deletes = append(s.deletes, name)
	return nil
}
func (s *stubSamba) SetGlobals(_ context.Context, opts GlobalsOpts) error {
	s.rec.record("smb.SetGlobals")
	if s.globalsErr != nil {
		return s.globalsErr
	}
	s.globals = append(s.globals, opts)
	return nil
}

// newStubs returns the trio with a shared recorder.
func newStubs() (*recorder, *stubDS, *stubNFS, *stubSamba) {
	r := &recorder{}
	return r, &stubDS{rec: r}, &stubNFS{rec: r}, &stubSamba{rec: r}
}

// ---------- helpers ----------

func sampleShare() ProtocolShare {
	return ProtocolShare{
		Name:        "media",
		Pool:        "tank",
		DatasetName: "media",
		Protocols:   []Protocol{ProtocolNFS, ProtocolSMB},
		ACLs: []ACE{
			{Type: dataset.ACETypeAllow, Principal: "OWNER@", Permissions: []dataset.ACLPerm{dataset.PermFullControl}},
		},
		QuotaBytes: 1 << 30,
		NFS: &NFSOpts{Clients: []nfs.ClientRule{
			{Spec: "10.0.0.0/24", Options: "rw,sync,sec=sys"},
		}},
		SMB: &SMBOpts{
			Comment:    "media share",
			Browseable: true,
			ValidUsers: []string{"alice"},
		},
	}
}

// expectContainsInOrder asserts that all of `want` appear in `got` in
// the given relative order (allowing other entries in between).
func expectContainsInOrder(t *testing.T, got []string, want ...string) {
	t.Helper()
	i := 0
	for _, g := range got {
		if i < len(want) && g == want[i] {
			i++
		}
	}
	if i != len(want) {
		t.Errorf("expected ordered subsequence %v in %v (matched %d)", want, got, i)
	}
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// ---------- tests ----------

func TestCreate_HappyPath(t *testing.T) {
	r, ds, n, sb := newStubs()
	m := New(ds, n, sb)
	ctx := context.Background()

	if err := m.Create(ctx, sampleShare()); err != nil {
		t.Fatalf("Create: %v", err)
	}

	// dataset created with managed properties + quota.
	if len(ds.creates) != 1 {
		t.Fatalf("expected 1 dataset create, got %d", len(ds.creates))
	}
	spec := ds.creates[0]
	if spec.Parent != "tank" || spec.Name != "media" || spec.Type != "filesystem" {
		t.Errorf("unexpected spec: %+v", spec)
	}
	wantProps := map[string]string{
		"acltype":         "nfsv4",
		"aclmode":         "passthrough",
		"aclinherit":      "passthrough",
		"xattr":           "sa",
		"casesensitivity": "mixed",
		"utf8only":        "on",
		"normalization":   "formD",
		"quota":           "1073741824",
	}
	if !reflect.DeepEqual(spec.Properties, wantProps) {
		t.Errorf("properties:\n got %+v\nwant %+v", spec.Properties, wantProps)
	}

	// ACL set against the dataset's path.
	if len(ds.setACLs) != 1 || ds.setACLs[0].path != "/tank/media" {
		t.Errorf("ACL not applied to /tank/media: %+v", ds.setACLs)
	}

	// NFS export and Samba share created with the right name and path.
	if len(n.creates) != 1 || n.creates[0].Name != "media" || n.creates[0].Path != "/tank/media" {
		t.Errorf("nfs create wrong: %+v", n.creates)
	}
	if len(sb.creates) != 1 || sb.creates[0].Name != "media" || sb.creates[0].Path != "/tank/media" {
		t.Errorf("smb create wrong: %+v", sb.creates)
	}

	// Order: ds.Create → ds.SetACL → nfs.CreateExport → smb.CreateShare.
	expectContainsInOrder(t, r.calls,
		"ds.Create", "ds.SetACL", "nfs.CreateExport", "smb.CreateShare")
}

func TestCreate_RollbackOnSambaFailure(t *testing.T) {
	r, ds, n, sb := newStubs()
	sb.createErr = errors.New("samba boom")
	m := New(ds, n, sb)

	if err := m.Create(context.Background(), sampleShare()); err == nil {
		t.Fatal("expected error")
	}
	if len(n.deletes) != 1 || n.deletes[0] != "media" {
		t.Errorf("expected nfs DeleteExport(media), got %+v", n.deletes)
	}
	if len(ds.destroys) != 1 || ds.destroys[0].name != "tank/media" {
		t.Errorf("expected ds.Destroy(tank/media), got %+v", ds.destroys)
	}
	expectContainsInOrder(t, r.calls,
		"ds.Create", "ds.SetACL", "nfs.CreateExport",
		"smb.CreateShare", "nfs.DeleteExport", "ds.Destroy")
}

func TestCreate_RollbackOnACLFailure(t *testing.T) {
	_, ds, n, sb := newStubs()
	ds.setACLErr = errors.New("acl boom")
	m := New(ds, n, sb)
	if err := m.Create(context.Background(), sampleShare()); err == nil {
		t.Fatal("expected error")
	}
	if len(ds.destroys) != 1 || ds.destroys[0].name != "tank/media" {
		t.Errorf("expected ds.Destroy on ACL failure, got %+v", ds.destroys)
	}
	if len(n.creates) != 0 || len(sb.creates) != 0 {
		t.Errorf("nfs/samba should not have been touched; got nfs=%v smb=%v", n.creates, sb.creates)
	}
}

func TestDelete_HappyPath_ReverseOrder(t *testing.T) {
	r, ds, n, sb := newStubs()
	m := New(ds, n, sb)
	if err := m.DeleteShare(context.Background(), sampleShare()); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if len(sb.deletes) != 1 || sb.deletes[0] != "media" {
		t.Errorf("smb delete wrong: %+v", sb.deletes)
	}
	if len(n.deletes) != 1 || n.deletes[0] != "media" {
		t.Errorf("nfs delete wrong: %+v", n.deletes)
	}
	if len(ds.destroys) != 1 || ds.destroys[0].name != "tank/media" {
		t.Errorf("ds destroy wrong: %+v", ds.destroys)
	}
	expectContainsInOrder(t, r.calls,
		"smb.DeleteShare", "nfs.DeleteExport", "ds.Destroy")
}

func TestDelete_PartialFailures_MultiError(t *testing.T) {
	_, ds, n, sb := newStubs()
	ds.destroyErr = errors.New("ds boom")
	n.deleteErr = errors.New("nfs boom")
	sb.deleteErr = samba.ErrNotFound // tolerated
	m := New(ds, n, sb)
	err := m.DeleteShare(context.Background(), sampleShare())
	if err == nil {
		t.Fatal("expected multi-error")
	}
	msg := err.Error()
	if !contains(msg, "nfs boom") || !contains(msg, "ds boom") {
		t.Errorf("expected combined error, got %v", err)
	}
}

func TestUpdate_CreatesWhenMissing(t *testing.T) {
	_, ds, n, sb := newStubs()
	ds.gets = map[string]*dataset.Detail{}
	ds.getErr = map[string]error{"tank/media": dataset.ErrNotFound}
	n.updateErr = nfs.ErrNotFound
	sb.updateErr = samba.ErrNotFound
	m := New(ds, n, sb)
	if err := m.Update(context.Background(), sampleShare()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(ds.creates) != 1 {
		t.Errorf("expected dataset create, got %d", len(ds.creates))
	}
	if len(n.creates) != 1 {
		t.Errorf("expected nfs create, got %d", len(n.creates))
	}
	if len(sb.creates) != 1 {
		t.Errorf("expected samba create, got %d", len(sb.creates))
	}
}

func TestUpdate_UpdatesWhenPresent(t *testing.T) {
	_, ds, n, sb := newStubs()
	ds.gets = map[string]*dataset.Detail{"tank/media": {}}
	m := New(ds, n, sb)
	if err := m.Update(context.Background(), sampleShare()); err != nil {
		t.Fatalf("Update: %v", err)
	}
	if len(ds.creates) != 0 {
		t.Errorf("dataset should not be created")
	}
	if len(ds.setProps) != 1 {
		t.Errorf("expected SetProps to converge ZFS properties, got %d", len(ds.setProps))
	}
	if len(n.updates) != 1 {
		t.Errorf("expected nfs update, got %d", len(n.updates))
	}
	if len(sb.updates) != 1 {
		t.Errorf("expected samba update, got %d", len(sb.updates))
	}
	if len(ds.setACLs) != 1 {
		t.Errorf("ACL should be reapplied, got %d", len(ds.setACLs))
	}
}

func TestGet_ReturnsActiveStateForBothProtocols(t *testing.T) {
	tdir := t.TempDir()
	nfsDir := tdir + "/nfs"
	smbDir := tdir + "/smb"
	if err := os.MkdirAll(nfsDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(smbDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(nfsDir+"/nova-nas-media.exports", []byte("stub"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Intentionally do NOT create the samba file → smb is inactive.

	_, ds, n, sb := newStubs()
	ds.gets = map[string]*dataset.Detail{"tank/media": {}}
	ds.getACL = []ACE{{Type: dataset.ACETypeAllow, Principal: "OWNER@", Permissions: []dataset.ACLPerm{dataset.PermRead, dataset.PermWrite, dataset.PermExecute}}}

	m := &Manager{
		Datasets:       ds,
		NFS:            n,
		Samba:          sb,
		NFSExportsDir:  nfsDir,
		SambaConfigDir: smbDir,
	}

	det, err := m.Get(context.Background(), sampleShare())
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if det.Path != "/tank/media" {
		t.Errorf("path wrong: %s", det.Path)
	}
	if len(det.ACL) != 1 {
		t.Errorf("ACL passthrough wrong: %+v", det.ACL)
	}
	if len(det.Protocols) != 2 {
		t.Fatalf("expected 2 protocol statuses, got %d", len(det.Protocols))
	}
	var nfsActive, smbActive bool
	for _, p := range det.Protocols {
		switch p.Protocol {
		case ProtocolNFS:
			nfsActive = p.Active
		case ProtocolSMB:
			smbActive = p.Active
		}
	}
	if !nfsActive {
		t.Errorf("nfs should be active")
	}
	if smbActive {
		t.Errorf("smb should be inactive")
	}
}

func TestInitGlobals_DelegatesToSamba(t *testing.T) {
	_, ds, n, sb := newStubs()
	m := New(ds, n, sb)
	opts := GlobalsOpts{ACLProfile: "nfsv4"}
	if err := m.InitGlobals(context.Background(), opts); err != nil {
		t.Fatalf("InitGlobals: %v", err)
	}
	if len(sb.globals) != 1 || !reflect.DeepEqual(sb.globals[0], opts) {
		t.Errorf("SetGlobals not forwarded: %+v", sb.globals)
	}
}

func TestValidate_RejectsBadInput(t *testing.T) {
	_, ds, n, sb := newStubs()
	m := New(ds, n, sb)
	cases := []struct {
		name  string
		share ProtocolShare
	}{
		{"empty name", ProtocolShare{Pool: "tank", DatasetName: "x", Protocols: []Protocol{ProtocolNFS}, ACLs: []ACE{{Type: dataset.ACETypeAllow, Principal: "OWNER@", Permissions: []dataset.ACLPerm{dataset.PermFullControl}}}}},
		{"bad name", ProtocolShare{Name: "bad name", Pool: "tank", DatasetName: "x", Protocols: []Protocol{ProtocolNFS}, ACLs: []ACE{{Type: dataset.ACETypeAllow, Principal: "OWNER@", Permissions: []dataset.ACLPerm{dataset.PermFullControl}}}}},
		{"no pool", ProtocolShare{Name: "x", DatasetName: "x", Protocols: []Protocol{ProtocolNFS}, ACLs: []ACE{{Type: dataset.ACETypeAllow, Principal: "OWNER@", Permissions: []dataset.ACLPerm{dataset.PermFullControl}}}}},
		{"no datasetName", ProtocolShare{Name: "x", Pool: "tank", Protocols: []Protocol{ProtocolNFS}, ACLs: []ACE{{Type: dataset.ACETypeAllow, Principal: "OWNER@", Permissions: []dataset.ACLPerm{dataset.PermFullControl}}}}},
		{"no protocols", ProtocolShare{Name: "x", Pool: "tank", DatasetName: "x", ACLs: []ACE{{Type: dataset.ACETypeAllow, Principal: "OWNER@", Permissions: []dataset.ACLPerm{dataset.PermFullControl}}}}},
		{"unknown proto", ProtocolShare{Name: "x", Pool: "tank", DatasetName: "x", Protocols: []Protocol{"ftp"}, ACLs: []ACE{{Type: dataset.ACETypeAllow, Principal: "OWNER@", Permissions: []dataset.ACLPerm{dataset.PermFullControl}}}}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if err := m.Create(context.Background(), c.share); err == nil {
				t.Errorf("expected validation error for %s", c.name)
			}
		})
	}
}
