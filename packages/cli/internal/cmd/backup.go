package cmd

import "github.com/spf13/cobra"

func newBackupCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular:  "backup",
		Plural:    "backups",
		APIPath:   "/api/v1/backups",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
}
