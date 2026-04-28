package exec

import (
	"bytes"
	"context"
	"errors"
	"io"
	osexec "os/exec"
)

// StreamRunner is the function signature for executing a host command with
// streaming stdin/stdout, used by zfs send/receive where stdout (or stdin) is
// a binary stream rather than a buffered byte slice. nil for stdin or stdout
// is allowed (treated as no piping). Tests can override on a per-Manager basis.
type StreamRunner func(ctx context.Context, bin string, stdin io.Reader, stdout io.Writer, args ...string) error

// RunStream executes bin with the given args, piping stdin from r and
// stdout to w. stderr is captured into a buffer and surfaced via *HostError
// on non-zero exit. Either stdin or stdout may be nil.
func RunStream(ctx context.Context, bin string, stdin io.Reader, stdout io.Writer, args ...string) error {
	cmd := osexec.CommandContext(ctx, bin, args...)
	if stdin != nil {
		cmd.Stdin = stdin
	}
	if stdout != nil {
		cmd.Stdout = stdout
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		exitCode := 0
		var ee *osexec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		}
		return &HostError{
			Bin:      bin,
			Args:     args,
			ExitCode: exitCode,
			Stderr:   stderr.String(),
			Cause:    err,
		}
	}
	return nil
}

// Runner is the function signature for executing a host command. The
// default is the package-level Run function; tests can override on a
// per-Manager basis.
type Runner func(ctx context.Context, bin string, args ...string) ([]byte, error)

func Run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	cmd := osexec.CommandContext(ctx, bin, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		exitCode := 0
		var ee *osexec.ExitError
		if errors.As(err, &ee) {
			exitCode = ee.ExitCode()
		}
		return stdout.Bytes(), &HostError{
			Bin:      bin,
			Args:     args,
			ExitCode: exitCode,
			Stderr:   stderr.String(),
			Cause:    err,
		}
	}
	return stdout.Bytes(), nil
}
