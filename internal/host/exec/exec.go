package exec

import (
	"bytes"
	"context"
	"errors"
	osexec "os/exec"
)

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
