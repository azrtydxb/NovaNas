package cmd

import (
	"log/slog"
	"os"

	"github.com/spf13/cobra"
)

// GlobalFlags holds flags shared by all commands.
type GlobalFlags struct {
	Output                string
	Context               string
	Server                string
	InsecureSkipTLSVerify bool
	Token                 string
	Verbose               bool
}

// Globals is the package-wide pointer to parsed global flags.
var Globals = &GlobalFlags{}

// Logger is the structured logger used by commands.
var Logger *slog.Logger

// NewRootCommand constructs the root cobra command and wires subcommands.
func NewRootCommand() *cobra.Command {
	root := &cobra.Command{
		Use:   "novanasctl",
		Short: "Command-line interface for NovaNas",
		Long: `novanasctl is the command-line interface for NovaNas.

It talks to the NovaNas API (not kube-apiserver) and supports interactive
login via OIDC device-code flow or API tokens for scripting.`,
		SilenceUsage: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			level := slog.LevelInfo
			if Globals.Verbose {
				level = slog.LevelDebug
			}
			Logger = slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level}))
		},
	}

	root.PersistentFlags().StringVarP(&Globals.Output, "output", "o", "table", "output format: table|json|yaml")
	root.PersistentFlags().StringVar(&Globals.Context, "context", "", "named context from config to use")
	root.PersistentFlags().StringVar(&Globals.Server, "server", "", "override server URL")
	root.PersistentFlags().BoolVar(&Globals.InsecureSkipTLSVerify, "insecure-skip-tls-verify", false, "skip TLS certificate verification")
	root.PersistentFlags().StringVar(&Globals.Token, "token", "", "API token (bypasses stored credentials)")
	root.PersistentFlags().BoolVarP(&Globals.Verbose, "verbose", "v", false, "verbose logging")

	root.AddCommand(
		newVersionCmd(),
		newLoginCmd(),
		newLogoutCmd(),
		newWhoamiCmd(),
		newContextCmd(),
		newPoolCmd(),
		newDatasetCmd(),
		newBucketCmd(),
		newShareCmd(),
		newDiskCmd(),
		newSnapshotCmd(),
		newReplicationCmd(),
		newBackupCmd(),
		newAppCmd(),
		newVMCmd(),
		newUserCmd(),
		newSystemCmd(),
		newCompletionCmd(),
	)

	return root
}
