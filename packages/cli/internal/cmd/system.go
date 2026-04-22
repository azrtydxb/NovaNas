package cmd

import (
	"encoding/json"
	"io"

	"github.com/azrtydxb/novanas/packages/cli/internal/output"
	"github.com/spf13/cobra"
)

func newSystemCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "system",
		Short: "System-level operations (health, info, upgrade)",
	}

	root.AddCommand(&cobra.Command{
		Use:   "info",
		Short: "Show server info",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := c.Do("GET", "/api/v1/system/info", nil, &raw); err != nil {
				return handleAPIError(err)
			}
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			return emit(m, func(w io.Writer) {
				rows := [][]string{}
				for k, v := range m {
					rows = append(rows, []string{k, stringAt(map[string]any{"v": v}, "v")})
				}
				output.Table(w, []string{"KEY", "VALUE"}, rows)
			})
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "health",
		Short: "Show server health",
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := c.Do("GET", "/api/v1/system/health", nil, &raw); err != nil {
				return handleAPIError(err)
			}
			var m map[string]any
			_ = json.Unmarshal(raw, &m)
			return emit(m, nil)
		},
	})

	return root
}
