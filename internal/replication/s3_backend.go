package replication

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// S3Object is a minimal description of an object encountered while
// listing a bucket prefix. Real implementations only need to populate
// the fields they support.
type S3Object struct {
	Key  string
	Size int64
}

// S3Client is the narrow surface the S3 backend uses against an
// S3-compatible store. It is intentionally tiny so we can ship a fake
// client for tests without depending on aws-sdk-go-v2.
//
// A production implementation can be built on aws-sdk-go-v2 (RustFS,
// MinIO, AWS, Wasabi all support these primitives). Adding the SDK is
// purposely deferred to a later change so this package compiles cleanly
// today; see docs/replication/README.md for the integration plan.
type S3Client interface {
	// PutObject uploads body to bucket/key. size may be -1 if unknown.
	PutObject(ctx context.Context, bucket, key string, body io.Reader, size int64) error
	// GetObject downloads bucket/key. The caller is responsible for
	// closing the returned reader.
	GetObject(ctx context.Context, bucket, key string) (io.ReadCloser, error)
	// ListObjects enumerates objects under bucket/prefix.
	ListObjects(ctx context.Context, bucket, prefix string) ([]S3Object, error)
	// DeleteObject removes a single object.
	DeleteObject(ctx context.Context, bucket, key string) error
}

// S3Backend implements Backend for push (mount → walk → PUT) and pull
// (LIST + GET → write to local mount). Versioning + lifecycle are
// configured on the bucket itself; this backend simply uses the plain
// PUT/GET/LIST/DELETE primitives.
//
// Credentials live in OpenBao under Job.SecretRef and are supplied to
// the S3Client at construction time by the caller (typically in
// cmd/nova-api/main.go); the backend never reaches into a secret store
// directly.
type S3Backend struct {
	Client S3Client
	// FS is the filesystem root used to walk the local source on push
	// and write files back on pull. When nil the operating-system
	// filesystem is used. Tests can supply a fake.
	FS S3FS
}

// S3FS is the filesystem abstraction used by the S3 backend. It is a
// strict subset of os.* with WriteFile/ReadFile/Walk semantics.
type S3FS interface {
	WalkDir(root string, fn fs.WalkDirFunc) error
	Open(name string) (io.ReadCloser, error)
	Create(name string) (io.WriteCloser, error)
	MkdirAll(path string, perm fs.FileMode) error
	Stat(name string) (fs.FileInfo, error)
}

// OSFS is the default S3FS, backed by the host filesystem.
type OSFS struct{}

func (OSFS) WalkDir(root string, fn fs.WalkDirFunc) error { return filepath.WalkDir(root, fn) }
func (OSFS) Open(name string) (io.ReadCloser, error)      { return os.Open(name) }
func (OSFS) Create(name string) (io.WriteCloser, error)   { return os.Create(name) }
func (OSFS) MkdirAll(p string, perm fs.FileMode) error    { return os.MkdirAll(p, perm) }
func (OSFS) Stat(name string) (fs.FileInfo, error)        { return os.Stat(name) }

// Kind implements Backend.
func (b *S3Backend) Kind() BackendKind { return BackendS3 }

// Validate implements Backend.
func (b *S3Backend) Validate(_ context.Context, j Job) error {
	if j.Direction == DirectionPush {
		if j.Source.Path == "" {
			return errors.New("s3 push: source.path is required (mount the dataset first)")
		}
		if j.Destination.Bucket == "" {
			return errors.New("s3 push: destination.bucket is required")
		}
	} else {
		if j.Source.Bucket == "" {
			return errors.New("s3 pull: source.bucket is required")
		}
		if j.Destination.Path == "" {
			return errors.New("s3 pull: destination.path is required")
		}
	}
	return nil
}

// Execute implements Backend.
func (b *S3Backend) Execute(ctx context.Context, in ExecuteContext) (RunResult, error) {
	if b.Client == nil {
		return RunResult{}, errors.New("s3 backend: client is required")
	}
	fsys := b.FS
	if fsys == nil {
		fsys = OSFS{}
	}
	if in.Job.Direction == DirectionPush {
		return b.push(ctx, in.Job, fsys)
	}
	return b.pull(ctx, in.Job, fsys)
}

func (b *S3Backend) push(ctx context.Context, j Job, fsys S3FS) (RunResult, error) {
	root := j.Source.Path
	bucket := j.Destination.Bucket
	prefix := strings.Trim(j.Destination.Prefix, "/")
	var total int64
	walkErr := fsys.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, rerr := filepath.Rel(root, path)
		if rerr != nil {
			return rerr
		}
		key := rel
		if prefix != "" {
			key = prefix + "/" + filepath.ToSlash(rel)
		} else {
			key = filepath.ToSlash(rel)
		}
		f, oerr := fsys.Open(path)
		if oerr != nil {
			return oerr
		}
		defer func() { _ = f.Close() }()
		st, serr := fsys.Stat(path)
		if serr != nil {
			return serr
		}
		if perr := b.Client.PutObject(ctx, bucket, key, f, st.Size()); perr != nil {
			return fmt.Errorf("put %s: %w", key, perr)
		}
		total += st.Size()
		return nil
	})
	if walkErr != nil {
		return RunResult{BytesTransferred: total}, walkErr
	}
	return RunResult{BytesTransferred: total}, nil
}

func (b *S3Backend) pull(ctx context.Context, j Job, fsys S3FS) (RunResult, error) {
	bucket := j.Source.Bucket
	prefix := strings.Trim(j.Source.Prefix, "/")
	root := j.Destination.Path
	objs, err := b.Client.ListObjects(ctx, bucket, prefix)
	if err != nil {
		return RunResult{}, err
	}
	var total int64
	for _, o := range objs {
		key := o.Key
		rel := strings.TrimPrefix(key, prefix)
		rel = strings.TrimPrefix(rel, "/")
		dst := filepath.Join(root, filepath.FromSlash(rel))
		if mkErr := fsys.MkdirAll(filepath.Dir(dst), 0o755); mkErr != nil {
			return RunResult{BytesTransferred: total}, mkErr
		}
		body, gerr := b.Client.GetObject(ctx, bucket, key)
		if gerr != nil {
			return RunResult{BytesTransferred: total}, gerr
		}
		f, cerr := fsys.Create(dst)
		if cerr != nil {
			_ = body.Close()
			return RunResult{BytesTransferred: total}, cerr
		}
		n, cpErr := io.Copy(f, body)
		_ = body.Close()
		_ = f.Close()
		if cpErr != nil {
			return RunResult{BytesTransferred: total}, cpErr
		}
		total += n
	}
	return RunResult{BytesTransferred: total}, nil
}
