package cmd

import "github.com/spf13/cobra"

func newShareCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular:  "share",
		Plural:    "shares",
		APIPath:   "/api/v1/shares",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
}
