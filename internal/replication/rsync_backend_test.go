package replication

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type fakeRsync struct {
	gotArgs []string
	bytes   int64
	err     error
}

func (f *fakeRsync) Run(_ context.Context, args []string) (int64, error) {
	f.gotArgs = args
	return f.bytes, f.err
}

func TestRsyncValidate(t *testing.T) {
	b := &RsyncBackend{Runner: &fakeRsync{}}
	if err := b.Validate(context.Background(), Job{Direction: DirectionPush}); err == nil {
		t.Fatal("expected missing source.path error")
	}
	if err := b.Validate(context.Background(), Job{
		Direction:   DirectionPush,
		Source:      Source{Path: "/srv"},
		Destination: Destination{Host: "h", SSHUser: "u", Path: "/dst"},
	}); err != nil {
		t.Fatalf("unexpected: %v", err)
	}
}

func TestRsyncBuildArgsPushWithKey(t *testing.T) {
	r := &fakeRsync{}
	b := &RsyncBackend{Runner: r, KeyPath: "/tmp/k"}
	job := Job{
		Direction:   DirectionPush,
		Source:      Source{Path: "/srv/data/"},
		Destination: Destination{Host: "h.example", SSHUser: "nova", Path: "/dst/data/"},
	}
	if _, err := b.Execute(context.Background(), ExecuteContext{Job: job}); err != nil {
		t.Fatalf("Execute: %v", err)
	}
	want := []string{
		"-aAXH", "--delete", "--stats",
		"-e", "ssh -i /tmp/k -o StrictHostKeyChecking=accept-new",
		"/srv/data/", "nova@h.example:/dst/data/",
	}
	if !reflect.DeepEqual(r.gotArgs, want) {
		t.Fatalf("args=%v\nwant %v", r.gotArgs, want)
	}
}

func TestRsyncRunPropagatesError(t *testing.T) {
	want := errors.New("rsync exit 23")
	b := &RsyncBackend{Runner: &fakeRsync{err: want}}
	_, err := b.Execute(context.Background(), ExecuteContext{Job: Job{
		Direction: DirectionPush, Source: Source{Path: "/a"},
		Destination: Destination{Host: "h", SSHUser: "u", Path: "/b"},
	}})
	if err == nil || !errors.Is(err, want) {
		t.Fatalf("err=%v want wraps %v", err, want)
	}
}

func TestParseRsyncBytes(t *testing.T) {
	out := "Number of files: 5\nTotal bytes sent: 1,234,567\n"
	if got := parseRsyncBytes(out); got != 1234567 {
		t.Fatalf("got %d", got)
	}
	if got := parseRsyncBytes("nope"); got != 0 {
		t.Fatalf("got %d want 0", got)
	}
}
