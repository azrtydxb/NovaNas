package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/azrtydxb/novanas/packages/cli/internal/cmd"
)

func TestRootCommandBuilds(t *testing.T) {
	root := cmd.NewRootCommand()
	if root == nil {
		t.Fatal("nil root")
	}
	if root.Use != "novanasctl" {
		t.Fatalf("unexpected Use: %q", root.Use)
	}
}

func TestHelpRuns(t *testing.T) {
	root := cmd.NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute help: %v", err)
	}
	if !strings.Contains(buf.String(), "novanasctl") {
		t.Fatalf("help missing program name:\n%s", buf.String())
	}
}

func TestVersionRuns(t *testing.T) {
	root := cmd.NewRootCommand()
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetErr(&buf)
	root.SetArgs([]string{"version"})
	if err := root.Execute(); err != nil {
		t.Fatalf("execute version: %v", err)
	}
	if !strings.Contains(buf.String(), "novanasctl") {
		t.Fatalf("version output missing program name:\n%s", buf.String())
	}
}
