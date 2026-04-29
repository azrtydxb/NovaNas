package csi

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

// notFoundErr is the sentinel returned by fakeClient when a dataset/snapshot
// is missing. It satisfies fakeClient.IsNotFound.
type notFoundErr struct{ what string }

func (e notFoundErr) Error() string { return fmt.Sprintf("not found: %s", e.what) }

// fakeClient is an in-memory implementation of NovaNASClient that records
// calls made by the CSI services.
type fakeClient struct {
	mu        sync.Mutex
	datasets  map[string]*Dataset
	snapshots map[string]struct{}
	shares    map[string]*ProtocolShareDetail // key = "<pool>/<datasetName>"

	// Captured spec for the last CreateDataset call.
	LastCreate     CreateDatasetSpec
	CreateCount    int
	LastSetProps   map[string]string
	LastSetTarget  string
	LastSnapSrc    string
	LastSnapShort  string
	LastDestroyDS  string
	LastDestroySn  string
	LastShareSpec  ProtocolShareSpec
	ShareCreateCnt int
	ShareDeleteCnt int
	NextJobID      int
	WaitJobCalls   int
	GetDatasetErrs map[string]error
}

func newFakeClient() *fakeClient {
	return &fakeClient{
		datasets:       map[string]*Dataset{},
		snapshots:      map[string]struct{}{},
		shares:         map[string]*ProtocolShareDetail{},
		GetDatasetErrs: map[string]error{},
	}
}

func (f *fakeClient) shareKey(pool, dataset string) string {
	return pool + "/" + dataset
}

func (f *fakeClient) CreateProtocolShare(ctx context.Context, share ProtocolShareSpec) (*Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.LastShareSpec = share
	f.ShareCreateCnt++
	key := f.shareKey(share.Pool, share.DatasetName)
	if _, exists := f.shares[key]; exists {
		return nil, fmt.Errorf("already exists: %s", key)
	}
	f.shares[key] = &ProtocolShareDetail{
		Name:        share.Name,
		Pool:        share.Pool,
		DatasetName: share.DatasetName,
		Path:        "/" + share.Pool + "/" + share.DatasetName,
	}
	// Mirror as a dataset so DeleteVolume's fallback logic also works.
	full := share.Pool + "/" + share.DatasetName
	f.datasets[full] = &Dataset{Name: full, Type: "filesystem", Mountpoint: "/" + full}
	return f.newJob(), nil
}

func (f *fakeClient) GetProtocolShare(ctx context.Context, name, pool, dataset string) (*ProtocolShareDetail, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	key := f.shareKey(pool, dataset)
	d, ok := f.shares[key]
	if !ok {
		return nil, notFoundErr{key}
	}
	cp := *d
	return &cp, nil
}

func (f *fakeClient) DeleteProtocolShare(ctx context.Context, name, pool, dataset string) (*Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.ShareDeleteCnt++
	key := f.shareKey(pool, dataset)
	if _, ok := f.shares[key]; !ok {
		return nil, notFoundErr{key}
	}
	delete(f.shares, key)
	delete(f.datasets, key)
	return f.newJob(), nil
}

func (f *fakeClient) IsNotFound(err error) bool {
	var n notFoundErr
	return errors.As(err, &n)
}

func (f *fakeClient) GetDataset(ctx context.Context, fullname string) (*Dataset, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if e, ok := f.GetDatasetErrs[fullname]; ok {
		return nil, e
	}
	ds, ok := f.datasets[fullname]
	if !ok {
		return nil, notFoundErr{fullname}
	}
	cp := *ds
	return &cp, nil
}

func (f *fakeClient) CreateDataset(ctx context.Context, spec CreateDatasetSpec) (*Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.LastCreate = spec
	f.CreateCount++
	full := spec.FullName()
	if _, exists := f.datasets[full]; exists {
		return nil, fmt.Errorf("already exists: %s", full)
	}
	ds := &Dataset{Name: full, Type: spec.Type, Mountpoint: "/" + full}
	if spec.Type == "volume" {
		ds.Volsize = spec.VolumeSizeBytes
	} else {
		// Pick up quota from properties if set.
		if q, ok := spec.Properties["quota"]; ok {
			fmt.Sscanf(q, "%d", &ds.Quota)
		}
	}
	f.datasets[full] = ds
	return f.newJob(), nil
}

func (f *fakeClient) DestroyDataset(ctx context.Context, fullname string, recursive bool) (*Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.LastDestroyDS = fullname
	if _, ok := f.datasets[fullname]; !ok {
		return nil, notFoundErr{fullname}
	}
	delete(f.datasets, fullname)
	return f.newJob(), nil
}

func (f *fakeClient) SetDatasetProps(ctx context.Context, fullname string, props map[string]string) (*Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.LastSetTarget = fullname
	f.LastSetProps = props
	ds, ok := f.datasets[fullname]
	if !ok {
		return nil, notFoundErr{fullname}
	}
	if v, ok := props["volsize"]; ok {
		fmt.Sscanf(v, "%d", &ds.Volsize)
	}
	if v, ok := props["quota"]; ok {
		fmt.Sscanf(v, "%d", &ds.Quota)
	}
	return f.newJob(), nil
}

func (f *fakeClient) CreateSnapshot(ctx context.Context, dataset, short string, recursive bool) (*Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.LastSnapSrc = dataset
	f.LastSnapShort = short
	if _, ok := f.datasets[dataset]; !ok {
		return nil, notFoundErr{dataset}
	}
	f.snapshots[dataset+"@"+short] = struct{}{}
	return f.newJob(), nil
}

func (f *fakeClient) DestroySnapshot(ctx context.Context, fullname string) (*Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.LastDestroySn = fullname
	if _, ok := f.snapshots[fullname]; !ok {
		return nil, notFoundErr{fullname}
	}
	delete(f.snapshots, fullname)
	return f.newJob(), nil
}

func (f *fakeClient) CloneSnapshot(ctx context.Context, snap, target string, props map[string]string) (*Job, error) {
	return f.newJob(), nil
}

func (f *fakeClient) WaitJob(ctx context.Context, id string, _ time.Duration) (*Job, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.WaitJobCalls++
	return &Job{ID: id, State: "done"}, nil
}

func (f *fakeClient) newJob() *Job {
	f.NextJobID++
	return &Job{ID: fmt.Sprintf("job-%d", f.NextJobID), State: "queued"}
}

// fakeMounter records mount/unmount/format calls.
type fakeMounter struct {
	BindCalls    []string // "src->tgt[,ro]"
	UnmountCalls []string
	MkfsCalls    []string // "fsType:device"
	GrowCalls    []string // "fsType:target:device"
	NFSCalls     []string // "server:export->target|opts"
	mounted      map[string]bool
	formatted    map[string]string // device -> fstype
	dirs         map[string]bool
	files        map[string]bool
}

func newFakeMounter() *fakeMounter {
	return &fakeMounter{
		mounted:   map[string]bool{},
		formatted: map[string]string{},
		dirs:      map[string]bool{},
		files:     map[string]bool{},
	}
}

func (f *fakeMounter) BindMount(source, target string, readonly bool) error {
	s := fmt.Sprintf("%s->%s", source, target)
	if readonly {
		s += ",ro"
	}
	f.BindCalls = append(f.BindCalls, s)
	f.mounted[target] = true
	return nil
}
func (f *fakeMounter) Unmount(target string) error {
	f.UnmountCalls = append(f.UnmountCalls, target)
	delete(f.mounted, target)
	return nil
}
func (f *fakeMounter) IsMounted(target string) (bool, error) { return f.mounted[target], nil }
func (f *fakeMounter) Mkfs(device, fsType string) error {
	f.MkfsCalls = append(f.MkfsCalls, fmt.Sprintf("%s:%s", fsType, device))
	f.formatted[device] = fsType
	return nil
}
func (f *fakeMounter) IsFormatted(device string) (bool, string, error) {
	if t, ok := f.formatted[device]; ok {
		return true, t, nil
	}
	return false, "", nil
}
func (f *fakeMounter) GrowFS(target, device, fsType string) error {
	f.GrowCalls = append(f.GrowCalls, fmt.Sprintf("%s:%s:%s", fsType, target, device))
	return nil
}
func (f *fakeMounter) EnsureDir(p string) error  { f.dirs[p] = true; return nil }
func (f *fakeMounter) EnsureFile(p string) error { f.files[p] = true; return nil }
func (f *fakeMounter) NFSMount(server, exportPath, target string, opts NFSMountOpts) error {
	options := opts.Options
	if options == "" {
		options = DefaultNFSMountOptions
	}
	if opts.ReadOnly {
		options += ",ro"
	}
	f.NFSCalls = append(f.NFSCalls, fmt.Sprintf("%s:%s->%s|%s", server, exportPath, target, options))
	f.mounted[target] = true
	return nil
}
