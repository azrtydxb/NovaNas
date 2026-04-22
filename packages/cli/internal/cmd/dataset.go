package cmd

import "github.com/spf13/cobra"

func newDatasetCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular:  "dataset",
		Plural:    "datasets",
		APIPath:   "/api/v1/datasets",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
}
