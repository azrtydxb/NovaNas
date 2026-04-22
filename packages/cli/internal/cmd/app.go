package cmd

import "github.com/spf13/cobra"

func newAppCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular:  "app",
		Plural:    "apps",
		APIPath:   "/api/v1/apps",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
}
