package cmd

import "github.com/spf13/cobra"

func newPoolCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular:  "pool",
		Plural:    "pools",
		APIPath:   "/api/v1/pools",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
}
