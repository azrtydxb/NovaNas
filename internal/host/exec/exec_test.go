package exec

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestRun_Success(t *testing.T) {
	out, err := Run(context.Background(), "/bin/echo", "hello")
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("stdout=%q", out)
	}
}

func TestRun_NonZeroExit(t *testing.T) {
	_, err := Run(context.Background(), "/bin/sh", "-c", "exit 1")
	if err == nil {
		t.Fatal("expected error")
	}
	var he *HostError
	if !errors.As(err, &he) {
		t.Fatalf("want *HostError, got %T", err)
	}
	if he.ExitCode == 0 {
		t.Errorf("ExitCode=0")
	}
}

func TestRun_Timeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	_, err := Run(ctx, "/bin/sleep", "5")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRun_BinaryNotFound(t *testing.T) {
	_, err := Run(context.Background(), "/nonexistent/binary")
	if err == nil {
		t.Fatal("expected error")
	}
}
