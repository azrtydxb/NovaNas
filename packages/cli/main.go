// Command novanasctl is the NovaNas CLI.
//
// novanasctl talks to the NovaNas API server (not kube-apiserver). It supports
// interactive login via OIDC device-code flow and API tokens for scripts.
package main

import (
	"fmt"
	"os"

	"github.com/azrtydxb/novanas/packages/cli/internal/cmd"
)

func main() {
	if err := cmd.NewRootCommand().Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
}
