package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/azrtydxb/novanas/packages/cli/internal/output"
	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"
)

// resourceDef describes a REST resource the CLI exposes as list/get/delete/create.
type resourceDef struct {
	Singular  string   // "pool"
	Plural    string   // "pools"
	APIPath   string   // "/api/v1/pools"
	Columns   []string // header names for table output
	Extractor func(m map[string]any) []string
}

func (r resourceDef) rowsFrom(items []map[string]any) [][]string {
	rows := make([][]string, 0, len(items))
	for _, it := range items {
		rows = append(rows, r.Extractor(it))
	}
	return rows
}

// newResourceCmd builds the common `list/get/delete/create -f` command set.
func newResourceCmd(r resourceDef) *cobra.Command {
	root := &cobra.Command{
		Use:   r.Singular,
		Short: fmt.Sprintf("Manage %s", r.Plural),
	}

	root.AddCommand(&cobra.Command{
		Use:   "list",
		Short: fmt.Sprintf("List %s", r.Plural),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			var raw json.RawMessage
			if err := c.Do("GET", r.APIPath, nil, &raw); err != nil {
				return handleAPIError(err)
			}
			items := decodeListItems(raw)
			return emit(items, func(w io.Writer) {
				output.Table(w, r.Columns, r.rowsFrom(items))
			})
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "get <name>",
		Short: fmt.Sprintf("Get a %s", r.Singular),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			var obj map[string]any
			if err := c.Do("GET", r.APIPath+"/"+args[0], nil, &obj); err != nil {
				return handleAPIError(err)
			}
			return emit(obj, func(w io.Writer) {
				output.Table(w, r.Columns, r.rowsFrom([]map[string]any{obj}))
			})
		},
	})

	root.AddCommand(&cobra.Command{
		Use:   "delete <name>",
		Short: fmt.Sprintf("Delete a %s", r.Singular),
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			c, err := resolveClient()
			if err != nil {
				return err
			}
			if err := c.Do("DELETE", r.APIPath+"/"+args[0], nil, nil); err != nil {
				return handleAPIError(err)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "%s/%s deleted\n", r.Singular, args[0])
			return nil
		},
	})

	var file string
	createCmd := &cobra.Command{
		Use:   "create",
		Short: fmt.Sprintf("Create a %s from a YAML/JSON file", r.Singular),
		RunE: func(cmd *cobra.Command, args []string) error {
			if file == "" {
				return errors.New("--file/-f is required")
			}
			data, err := os.ReadFile(file)
			if err != nil {
				return err
			}
			var body any
			if err := yaml.Unmarshal(data, &body); err != nil {
				return fmt.Errorf("parse %s: %w", file, err)
			}
			c, err := resolveClient()
			if err != nil {
				return err
			}
			var out map[string]any
			if err := c.Do("POST", r.APIPath, body, &out); err != nil {
				return handleAPIError(err)
			}
			return emit(out, nil)
		},
	}
	createCmd.Flags().StringVarP(&file, "file", "f", "", "manifest file (YAML or JSON)")
	_ = createCmd.MarkFlagRequired("file")
	root.AddCommand(createCmd)

	return root
}

// defaultExtractor pulls common metadata.name/status.phase-style fields.
func defaultExtractor(m map[string]any) []string {
	name := stringAt(m, "metadata", "name")
	if name == "" {
		name = stringAt(m, "name")
	}
	ns := stringAt(m, "metadata", "namespace")
	phase := stringAt(m, "status", "phase")
	if phase == "" {
		phase = stringAt(m, "status")
	}
	return []string{name, ns, phase}
}

func stringAt(m map[string]any, keys ...string) string {
	var cur any = m
	for _, k := range keys {
		mm, ok := cur.(map[string]any)
		if !ok {
			return ""
		}
		cur = mm[k]
	}
	switch v := cur.(type) {
	case string:
		return v
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", v)
	}
}
