// Command kubectl-novanas is a kubectl plugin entry point that wraps
// novanasctl. When installed somewhere on $PATH as "kubectl-novanas",
// kubectl will dispatch `kubectl novanas ...` invocations to this
// binary, forwarding the remaining arguments verbatim.
//
// The plugin intentionally is a thin wrapper: it passes KUBECONFIG and
// KUBECTL_PLUGINS_CALLER context through so subcommands that care about
// the kube-context (e.g., `kubectl novanas install`) can use them.
//
// Build:
//
//	go build -o bin/kubectl-novanas ./cmd/kubectl-novanas
//
// Install (copy the binary anywhere on PATH):
//
//	install -m 0755 bin/kubectl-novanas /usr/local/bin/kubectl-novanas
//	kubectl novanas --help
package main

import (
	"fmt"
	"os"

	"github.com/azrtydxb/novanas/packages/cli/internal/cmd"
)

// pluginEnvCaller is set by kubectl when the binary is invoked as a plugin.
// Its presence is used only to annotate the CLI's invocation path; we do
// not otherwise change behaviour.
const pluginEnvCaller = "KUBECTL_PLUGINS_CALLER"

func main() {
	if caller := os.Getenv(pluginEnvCaller); caller != "" {
		// Expose the caller so subcommands can log context. This is a
		// hint only; the CLI's own flags and env take precedence.
		_ = os.Setenv("NOVANAS_INVOKED_AS", "kubectl-plugin")
	}
	root := cmd.NewRootCommand()
	root.Use = "kubectl novanas"
	root.Short = "NovaNas kubectl plugin (thin wrapper around novanasctl)"
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
