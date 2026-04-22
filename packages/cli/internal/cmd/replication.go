package cmd

import "github.com/spf13/cobra"

func newReplicationCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular:  "replication",
		Plural:    "replications",
		APIPath:   "/api/v1/replications",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
}
