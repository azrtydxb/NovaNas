package replication

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

type fakeSnapper struct{ created []string }

func (f *fakeSnapper) Create(_ context.Context, ds, short string, _ bool) error {
	f.created = append(f.created, ds+"@"+short)
	return nil
}

type fakeSender struct {
	body            []byte
	gotIncremental  string
	gotSnapshot     string
	err             error
}

func (f *fakeSender) Send(_ context.Context, snap, incFrom string, w io.Writer) error {
	f.gotSnapshot = snap
	f.gotIncremental = incFrom
	if f.err != nil {
		return f.err
	}
	_, err := w.Write(f.body)
	return err
}

type fakeReceiver struct {
	gotHost, gotUser, gotDataset string
	read                         []byte
	err                          error
}

func (f *fakeReceiver) Receive(_ context.Context, host, user, ds string, r io.Reader) (int64, error) {
	f.gotHost, f.gotUser, f.gotDataset = host, user, ds
	if f.err != nil {
		return 0, f.err
	}
	buf := &bytes.Buffer{}
	n, err := io.Copy(buf, r)
	f.read = buf.Bytes()
	return n, err
}

func TestZFSBackendValidate(t *testing.T) {
	b := &ZFSBackend{}
	if err := b.Validate(context.Background(), Job{Direction: DirectionPush}); err == nil {
		t.Fatal("expected error on missing source.dataset")
	}
	if err := b.Validate(context.Background(), Job{
		Direction: DirectionPush, Source: Source{Dataset: "tank/x"},
	}); err == nil {
		t.Fatal("expected error on missing destination")
	}
	if err := b.Validate(context.Background(), Job{
		Direction:   DirectionPush,
		Source:      Source{Dataset: "tank/x"},
		Destination: Destination{Dataset: "back/x", Host: "h"},
	}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestZFSBackendExecuteFullSend(t *testing.T) {
	snap := &fakeSnapper{}
	send := &fakeSender{body: []byte("zfs-stream-bytes")}
	recv := &fakeReceiver{}
	b := &ZFSBackend{
		Snapshots: snap, Sender: send, Receiver: recv,
		Now: func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) },
	}
	job := Job{
		Backend: BackendZFS, Direction: DirectionPush,
		Source:      Source{Dataset: "tank/data"},
		Destination: Destination{Dataset: "backup/data", Host: "remote.local", SSHUser: "nova"},
	}
	res, err := b.Execute(context.Background(), ExecuteContext{Job: job})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if !strings.HasPrefix(res.Snapshot, "tank/data@repl-2026-04-29-1200") {
		t.Fatalf("Snapshot=%q", res.Snapshot)
	}
	if res.BytesTransferred != int64(len("zfs-stream-bytes")) {
		t.Fatalf("Bytes=%d", res.BytesTransferred)
	}
	if recv.gotHost != "remote.local" || recv.gotUser != "nova" || recv.gotDataset != "backup/data" {
		t.Fatalf("receiver got wrong args: host=%q user=%q ds=%q", recv.gotHost, recv.gotUser, recv.gotDataset)
	}
	if send.gotIncremental != "" {
		t.Fatalf("expected full send, got incremental from %q", send.gotIncremental)
	}
}

func TestZFSBackendExecuteIncremental(t *testing.T) {
	snap := &fakeSnapper{}
	send := &fakeSender{body: []byte("delta")}
	recv := &fakeReceiver{}
	b := &ZFSBackend{
		Snapshots: snap, Sender: send, Receiver: recv,
		Now: func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) },
	}
	job := Job{
		Backend: BackendZFS, Direction: DirectionPush,
		Source:       Source{Dataset: "tank/data"},
		Destination:  Destination{Dataset: "backup/data", Host: "h"},
		LastSnapshot: "tank/data@repl-2026-04-28-1200",
	}
	if _, err := b.Execute(context.Background(), ExecuteContext{Job: job}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if send.gotIncremental != "tank/data@repl-2026-04-28-1200" {
		t.Fatalf("incFrom=%q", send.gotIncremental)
	}
}

func TestZFSBackendExecuteSendError(t *testing.T) {
	b := &ZFSBackend{
		Snapshots: &fakeSnapper{},
		Sender:    &fakeSender{err: errors.New("boom")},
		Receiver:  &fakeReceiver{},
	}
	_, err := b.Execute(context.Background(), ExecuteContext{Job: Job{
		Direction:   DirectionPush,
		Source:      Source{Dataset: "tank/x"},
		Destination: Destination{Dataset: "back/x", Host: "h"},
	}})
	if err == nil {
		t.Fatal("expected error")
	}
}
