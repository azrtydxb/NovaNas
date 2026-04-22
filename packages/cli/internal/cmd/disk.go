package cmd

import "github.com/spf13/cobra"

func newDiskCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular:  "disk",
		Plural:    "disks",
		APIPath:   "/api/v1/disks",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
}
