package replication

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"
)

// ZFSSnapshotter is the subset of the snapshot manager the ZFS backend
// uses. Mocked in tests.
type ZFSSnapshotter interface {
	Create(ctx context.Context, dataset, short string, recursive bool) error
}

// ZFSSender wraps a streaming "zfs send" call. Implementations write
// the raw zfs-send stream to w and return on EOF or error.
type ZFSSender interface {
	Send(ctx context.Context, snapshot, incrementalFrom string, w io.Writer) error
}

// ZFSReceiver wraps the receive side. The default production
// implementation tunnels through SSH to a remote host running
// "zfs receive"; tests can substitute a buffer-counting fake.
type ZFSReceiver interface {
	Receive(ctx context.Context, host, sshUser, dstDataset string, r io.Reader) (bytesWritten int64, err error)
}

// ZFSBackend implements Backend for native zfs send | zfs receive.
//
// It tracks the last snapshot replicated per job (via Job.LastSnapshot)
// so subsequent runs can use incremental sends (zfs send -i). The
// caller is responsible for passing the updated Job back through Manager
// so that Job.LastSnapshot is persisted (the Manager does this
// automatically when Backend.Execute returns a non-empty
// RunResult.Snapshot).
type ZFSBackend struct {
	Snapshots ZFSSnapshotter
	Sender    ZFSSender
	Receiver  ZFSReceiver
	// SnapshotPrefix is the short-name prefix used when this backend
	// creates the per-run snapshot. Defaults to "repl".
	SnapshotPrefix string
	// Now is the time source used when synthesising snapshot short names.
	Now func() time.Time
}

// Kind implements Backend.
func (b *ZFSBackend) Kind() BackendKind { return BackendZFS }

// Validate implements Backend.
func (b *ZFSBackend) Validate(_ context.Context, j Job) error {
	if j.Direction == DirectionPush {
		if j.Source.Dataset == "" {
			return errors.New("zfs push: source.dataset is required")
		}
		if j.Destination.Dataset == "" || j.Destination.Host == "" {
			return errors.New("zfs push: destination.dataset and destination.host are required")
		}
	} else {
		if j.Destination.Dataset == "" {
			return errors.New("zfs pull: destination.dataset is required")
		}
		if j.Source.Dataset == "" || j.Source.Host == "" {
			return errors.New("zfs pull: source.dataset and source.host are required")
		}
	}
	return nil
}

// Execute implements Backend. It snapshots the source dataset, runs
// "zfs send" into "zfs receive" through the configured Receiver, and
// returns the new snapshot's full name on success.
func (b *ZFSBackend) Execute(ctx context.Context, in ExecuteContext) (RunResult, error) {
	if b.Sender == nil || b.Receiver == nil || b.Snapshots == nil {
		return RunResult{}, errors.New("zfs backend: sender, receiver and snapshots are required")
	}
	now := b.Now
	if now == nil {
		now = time.Now
	}
	prefix := b.SnapshotPrefix
	if prefix == "" {
		prefix = "repl"
	}

	job := in.Job
	srcDataset, dstHost, dstUser, dstDataset := zfsEndpoints(job)

	short := fmt.Sprintf("%s-%s", prefix, now().UTC().Format("2006-01-02-1504"))
	if err := b.Snapshots.Create(ctx, srcDataset, short, false); err != nil {
		return RunResult{}, fmt.Errorf("zfs backend: snapshot create: %w", err)
	}
	full := srcDataset + "@" + short

	pr, pw := io.Pipe()
	sendErrCh := make(chan error, 1)
	go func() {
		err := b.Sender.Send(ctx, full, job.LastSnapshot, pw)
		_ = pw.CloseWithError(err)
		sendErrCh <- err
	}()
	bytesWritten, recvErr := b.Receiver.Receive(ctx, dstHost, dstUser, dstDataset, pr)
	_ = pr.CloseWithError(recvErr)
	sendErr := <-sendErrCh

	if recvErr != nil {
		return RunResult{}, fmt.Errorf("zfs receive: %w", recvErr)
	}
	if sendErr != nil {
		return RunResult{}, fmt.Errorf("zfs send: %w", sendErr)
	}
	return RunResult{BytesTransferred: bytesWritten, Snapshot: full}, nil
}

// zfsEndpoints flattens a Job's Source/Destination into the four
// parameters Execute needs, accounting for direction. For push the
// "destination" is remote; for pull the "source" is remote and the
// receiver writes locally.
func zfsEndpoints(j Job) (srcDataset, dstHost, dstUser, dstDataset string) {
	if j.Direction == DirectionPush {
		return j.Source.Dataset, j.Destination.Host, j.Destination.SSHUser, j.Destination.Dataset
	}
	// Pull: receiver runs locally; "host" is empty so the Receiver impl
	// short-circuits to a local zfs receive.
	return j.Source.Dataset, "", "", j.Destination.Dataset
}

// remoteDataset is a small helper exposed for callers that want to
// derive a destination dataset path by joining a prefix with the
// basename of the source.
func remoteDataset(prefix, src string) string {
	base := src
	if i := strings.LastIndexByte(src, '/'); i >= 0 {
		base = src[i+1:]
	}
	return strings.TrimRight(prefix, "/") + "/" + base
}

// Avoid "imported and not used" if remoteDataset is not invoked from
// elsewhere in the package right now.
var _ = remoteDataset
