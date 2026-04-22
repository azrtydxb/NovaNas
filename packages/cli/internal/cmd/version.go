package cmd

import (
	"fmt"

	"github.com/spf13/cobra"
)

// Version is the binary version, set via -ldflags at build time.
var Version = "0.0.0-dev"

// Commit is the git commit hash, set via -ldflags at build time.
var Commit = "unknown"

// BuildDate is the ISO8601 build time, set via -ldflags at build time.
var BuildDate = "unknown"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print novanasctl version",
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Fprintf(cmd.OutOrStdout(), "novanasctl %s (commit %s, built %s)\n", Version, Commit, BuildDate)
			return nil
		},
	}
}
