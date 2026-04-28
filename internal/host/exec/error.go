// Package exec is the only place the API shells out to host commands.
package exec

import "fmt"

type HostError struct {
	Bin      string
	Args     []string
	ExitCode int
	Stderr   string
	Cause    error
}

func (e *HostError) Error() string {
	if e.Cause != nil && e.ExitCode == 0 {
		return fmt.Sprintf("exec %s: %v", e.Bin, e.Cause)
	}
	return fmt.Sprintf("exec %s exit=%d: %s", e.Bin, e.ExitCode, e.Stderr)
}

func (e *HostError) Unwrap() error { return e.Cause }
