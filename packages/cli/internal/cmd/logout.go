package cmd

import (
	"fmt"

	"github.com/azrtydxb/novanas/packages/cli/internal/client"
	"github.com/azrtydxb/novanas/packages/cli/internal/config"
	"github.com/spf13/cobra"
)

func newLogoutCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Remove stored credentials for the current or selected context",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.DefaultPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			name := Globals.Context
			if name == "" {
				name = cfg.CurrentContext
			}
			if name == "" {
				return fmt.Errorf("no context to log out of")
			}
			if err := client.DeleteRefreshToken(name); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "warning: keyring delete: %v\n", err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged out of context %q\n", name)
			return nil
		},
	}
}
