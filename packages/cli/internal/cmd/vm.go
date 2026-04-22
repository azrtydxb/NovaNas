package cmd

import "github.com/spf13/cobra"

func newVMCmd() *cobra.Command {
	c := newResourceCmd(resourceDef{
		Singular:  "vm",
		Plural:    "vms",
		APIPath:   "/api/v1/vms",
		Columns:   []string{"NAME", "NAMESPACE", "PHASE"},
		Extractor: defaultExtractor,
	})
	c.Aliases = []string{"virtualmachine"}
	return c
}
