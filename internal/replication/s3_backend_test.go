package replication

import (
	"bytes"
	"context"
	"io"
	"io/fs"
	"path/filepath"
	"testing"
	"time"
)

type fakeS3 struct {
	puts map[string][]byte
	objs []S3Object
	gets map[string][]byte
}

func newFakeS3() *fakeS3 {
	return &fakeS3{puts: map[string][]byte{}, gets: map[string][]byte{}}
}

func (f *fakeS3) PutObject(_ context.Context, bucket, key string, body io.Reader, _ int64) error {
	buf, _ := io.ReadAll(body)
	f.puts[bucket+"/"+key] = buf
	return nil
}

func (f *fakeS3) GetObject(_ context.Context, bucket, key string) (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(f.gets[bucket+"/"+key])), nil
}

func (f *fakeS3) ListObjects(_ context.Context, _ string, _ string) ([]S3Object, error) {
	return f.objs, nil
}

func (f *fakeS3) DeleteObject(_ context.Context, _, _ string) error { return nil }

// memFS is a simple in-memory S3FS used by S3 backend tests for both
// push (read source files) and pull (write destination files).
type memFS struct {
	files map[string][]byte
}

func newMemFS() *memFS { return &memFS{files: map[string][]byte{}} }

type memFileInfo struct {
	name string
	size int64
}

func (m memFileInfo) Name() string       { return m.name }
func (m memFileInfo) Size() int64        { return m.size }
func (m memFileInfo) Mode() fs.FileMode  { return 0o644 }
func (m memFileInfo) ModTime() time.Time { return time.Time{} }
func (m memFileInfo) IsDir() bool        { return false }
func (m memFileInfo) Sys() any           { return nil }

type dirEntry struct{ fi fs.FileInfo }

func (d dirEntry) Name() string               { return d.fi.Name() }
func (d dirEntry) IsDir() bool                { return false }
func (d dirEntry) Type() fs.FileMode          { return d.fi.Mode().Type() }
func (d dirEntry) Info() (fs.FileInfo, error) { return d.fi, nil }

func (m *memFS) WalkDir(root string, fn fs.WalkDirFunc) error {
	for path, body := range m.files {
		if rel, err := filepath.Rel(root, path); err == nil && !filepathHasDotDot(rel) {
			fi := memFileInfo{name: filepath.Base(path), size: int64(len(body))}
			if err := fn(path, dirEntry{fi: fi}, nil); err != nil {
				return err
			}
		}
	}
	return nil
}

func filepathHasDotDot(p string) bool {
	parts := filepath.SplitList(p)
	for _, x := range parts {
		if x == ".." {
			return true
		}
	}
	return false
}

func (m *memFS) Open(name string) (io.ReadCloser, error) {
	body := m.files[name]
	return io.NopCloser(bytes.NewReader(body)), nil
}

type memWriter struct {
	dst  *memFS
	name string
	buf  bytes.Buffer
}

func (m *memWriter) Write(p []byte) (int, error) { return m.buf.Write(p) }
func (m *memWriter) Close() error {
	m.dst.files[m.name] = append([]byte(nil), m.buf.Bytes()...)
	return nil
}

func (m *memFS) Create(name string) (io.WriteCloser, error) {
	return &memWriter{dst: m, name: name}, nil
}

func (m *memFS) MkdirAll(string, fs.FileMode) error { return nil }
func (m *memFS) Stat(name string) (fs.FileInfo, error) {
	return memFileInfo{name: filepath.Base(name), size: int64(len(m.files[name]))}, nil
}

func TestS3BackendValidate(t *testing.T) {
	b := &S3Backend{Client: newFakeS3()}
	if err := b.Validate(context.Background(), Job{Direction: DirectionPush}); err == nil {
		t.Fatal("expected error: missing source.path")
	}
	if err := b.Validate(context.Background(), Job{
		Direction: DirectionPush, Source: Source{Path: "/srv/data"},
		Destination: Destination{Bucket: "b"},
	}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestS3BackendPush(t *testing.T) {
	mfs := newMemFS()
	mfs.files["/srv/data/a.txt"] = []byte("hello")
	mfs.files["/srv/data/sub/b.txt"] = []byte("world!")
	cli := newFakeS3()
	b := &S3Backend{Client: cli, FS: mfs}
	job := Job{
		Backend: BackendS3, Direction: DirectionPush,
		Source:      Source{Path: "/srv/data"},
		Destination: Destination{Bucket: "snap", Prefix: "backup1"},
	}
	res, err := b.Execute(context.Background(), ExecuteContext{Job: job})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.BytesTransferred != int64(len("hello")+len("world!")) {
		t.Fatalf("Bytes=%d", res.BytesTransferred)
	}
	if string(cli.puts["snap/backup1/a.txt"]) != "hello" {
		t.Fatalf("missing/incorrect a.txt put: %v", cli.puts)
	}
}

func TestS3BackendPull(t *testing.T) {
	mfs := newMemFS()
	cli := newFakeS3()
	cli.objs = []S3Object{{Key: "p/a.txt", Size: 5}, {Key: "p/sub/b.txt", Size: 3}}
	cli.gets["b1/p/a.txt"] = []byte("hello")
	cli.gets["b1/p/sub/b.txt"] = []byte("foo")
	b := &S3Backend{Client: cli, FS: mfs}
	job := Job{
		Backend: BackendS3, Direction: DirectionPull,
		Source:      Source{Bucket: "b1", Prefix: "p"},
		Destination: Destination{Path: "/dst"},
	}
	res, err := b.Execute(context.Background(), ExecuteContext{Job: job})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if res.BytesTransferred != 8 {
		t.Fatalf("Bytes=%d", res.BytesTransferred)
	}
	if string(mfs.files["/dst/a.txt"]) != "hello" {
		t.Fatalf("a.txt body=%q", mfs.files["/dst/a.txt"])
	}
}
