package cmd

import "github.com/spf13/cobra"

func newSnapshotCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular:  "snapshot",
		Plural:    "snapshots",
		APIPath:   "/api/v1/snapshots",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
}
