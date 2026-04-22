package cmd

import (
	"io"

	"github.com/azrtydxb/novanas/packages/cli/internal/output"
	"github.com/spf13/cobra"
)

func newWhoamiCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "whoami",
		Short: "Print the currently authenticated identity",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			me, err := c.WhoAmI()
			if err != nil {
				return handleAPIError(err)
			}
			return emit(me, func(w io.Writer) {
				output.Table(w, []string{"SUBJECT", "USERNAME", "EMAIL"},
					[][]string{{me.Subject, me.Username, me.Email}})
			})
		},
	}
}
