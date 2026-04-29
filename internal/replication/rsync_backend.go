package replication

import (
	"context"
	"errors"
	"fmt"
	"strconv"
)

// RsyncRunner is the abstraction the rsync backend uses to invoke
// /usr/bin/rsync. The default implementation shells out via
// internal/host/exec; tests substitute a fake.
type RsyncRunner interface {
	// Run executes rsync with the given args. The implementation must
	// honor ctx cancellation. Returns bytes transferred (parsed from
	// rsync stats output) and any execution error.
	Run(ctx context.Context, args []string) (bytesTransferred int64, err error)
}

// RsyncBackend implements Backend by shelling out to rsync over SSH.
//
// The per-job SSH key is fetched from OpenBao under Job.SecretRef and
// written to a temporary file that rsync can consume via its -e
// "ssh -i <keyfile>" option. The caller is responsible for the secret
// lookup; this backend operates on a finalised set of rsync arguments.
type RsyncBackend struct {
	Runner RsyncRunner
	// KeyPath, when non-empty, is used as the SSH identity file for
	// rsync's -e option. Production callers populate this from the
	// secrets manager before invoking Execute.
	KeyPath string
}

// Kind implements Backend.
func (b *RsyncBackend) Kind() BackendKind { return BackendRsync }

// Validate implements Backend.
func (b *RsyncBackend) Validate(_ context.Context, j Job) error {
	if j.Direction == DirectionPush {
		if j.Source.Path == "" {
			return errors.New("rsync push: source.path is required")
		}
		if j.Destination.Host == "" || j.Destination.SSHUser == "" || j.Destination.Path == "" {
			return errors.New("rsync push: destination.{host,sshUser,path} are required")
		}
	} else {
		if j.Destination.Path == "" {
			return errors.New("rsync pull: destination.path is required")
		}
		if j.Source.Host == "" || j.Source.SSHUser == "" || j.Source.Path == "" {
			return errors.New("rsync pull: source.{host,sshUser,path} are required")
		}
	}
	return nil
}

// Execute implements Backend.
func (b *RsyncBackend) Execute(ctx context.Context, in ExecuteContext) (RunResult, error) {
	if b.Runner == nil {
		return RunResult{}, errors.New("rsync backend: runner is required")
	}
	args := b.buildArgs(in.Job)
	n, err := b.Runner.Run(ctx, args)
	if err != nil {
		return RunResult{BytesTransferred: n}, fmt.Errorf("rsync: %w", err)
	}
	return RunResult{BytesTransferred: n}, nil
}

// buildArgs constructs the rsync argv for one run. We use -aAXH to
// preserve POSIX ACLs/xattrs/hardlinks, --delete to make the
// destination an exact mirror, --stats so the runner can parse a byte
// count from stdout, and --rsh="ssh -i <key>" when a key is provided.
//
// Idempotency: rsync is naturally idempotent for the file-tree mode
// used here. Re-running a successful job is a no-op except for
// --delete'd files at the destination.
func (b *RsyncBackend) buildArgs(j Job) []string {
	args := []string{"-aAXH", "--delete", "--stats"}
	if b.KeyPath != "" {
		args = append(args, "-e", "ssh -i "+b.KeyPath+" -o StrictHostKeyChecking=accept-new")
	}
	switch j.Direction {
	case DirectionPush:
		args = append(args, j.Source.Path,
			j.Destination.SSHUser+"@"+j.Destination.Host+":"+j.Destination.Path)
	case DirectionPull:
		args = append(args,
			j.Source.SSHUser+"@"+j.Source.Host+":"+j.Source.Path,
			j.Destination.Path)
	}
	return args
}

// parseRsyncBytes is a helper for runner implementations that have
// rsync's --stats stdout. It looks for the "Total bytes sent: N" line
// and returns N, or 0 if not found.
func parseRsyncBytes(stdout string) int64 {
	const marker = "Total bytes sent: "
	for {
		idx := indexOf(stdout, marker)
		if idx < 0 {
			return 0
		}
		stdout = stdout[idx+len(marker):]
		end := indexOf(stdout, "\n")
		if end < 0 {
			end = len(stdout)
		}
		raw := trimCommas(stdout[:end])
		n, err := strconv.ParseInt(raw, 10, 64)
		if err != nil {
			return 0
		}
		return n
	}
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func trimCommas(s string) string {
	out := make([]byte, 0, len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == ',' || s[i] == ' ' {
			continue
		}
		out = append(out, s[i])
	}
	return string(out)
}

// Suppress unused-warnings if parseRsyncBytes is consumed only by an
// out-of-tree runner.
var _ = parseRsyncBytes
