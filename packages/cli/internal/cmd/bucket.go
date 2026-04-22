package cmd

import "github.com/spf13/cobra"

func newBucketCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular:  "bucket",
		Plural:    "buckets",
		APIPath:   "/api/v1/buckets",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
}
