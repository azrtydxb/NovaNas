package cmd

import (
	"fmt"
	"io"

	"github.com/azrtydxb/novanas/packages/cli/internal/config"
	"github.com/azrtydxb/novanas/packages/cli/internal/output"
	"github.com/spf13/cobra"
)

func newContextCmd() *cobra.Command {
	root := &cobra.Command{
		Use:     "context",
		Aliases: []string{"ctx"},
		Short:   "Manage connection contexts",
	}

	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "List configured contexts",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.DefaultPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			return emit(cfg, func(w io.Writer) {
				rows := [][]string{}
				for _, c := range cfg.Contexts {
					mark := ""
					if c.Name == cfg.CurrentContext {
						mark = "*"
					}
					rows = append(rows, []string{mark, c.Name, c.Server})
				}
				output.Table(w, []string{"CURRENT", "NAME", "SERVER"}, rows)
			})
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "use <name>",
		Short: "Set the current context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.DefaultPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			found := false
			for _, c := range cfg.Contexts {
				if c.Name == args[0] {
					found = true
					break
				}
			}
			if !found {
				return fmt.Errorf("no such context %q", args[0])
			}
			cfg.CurrentContext = args[0]
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Switched to context %q\n", args[0])
			return nil
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "remove <name>",
		Short: "Remove a context",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := config.DefaultPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			out := cfg.Contexts[:0]
			for _, c := range cfg.Contexts {
				if c.Name != args[0] {
					out = append(out, c)
				}
			}
			cfg.Contexts = out
			if cfg.CurrentContext == args[0] {
				cfg.CurrentContext = ""
			}
			return config.Save(path, cfg)
		},
	})

	return root
}
