package cmd

import "github.com/spf13/cobra"

func newUserCmd() *cobra.Command {
	return newResourceCmd(resourceDef{
		Singular: "user",
		Plural:   "users",
		APIPath:  "/api/v1/users",
		Columns:  []string{"USERNAME", "EMAIL", "GROUPS"},
		Extractor: func(m map[string]any) []string {
			return []string{
				stringAt(m, "username"),
				stringAt(m, "email"),
				stringAt(m, "groups"),
			}
		},
	})
}
