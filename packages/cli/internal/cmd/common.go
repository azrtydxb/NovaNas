package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/azrtydxb/novanas/packages/cli/internal/client"
	"github.com/azrtydxb/novanas/packages/cli/internal/config"
	"github.com/azrtydxb/novanas/packages/cli/internal/output"
)

// resolveClient builds an authenticated API client using global flags and the
// config file. Precedence: --server > --context > current-context in config.
func resolveClient() (*client.Client, error) {
	server := Globals.Server
	insecure := Globals.InsecureSkipTLSVerify
	contextName := Globals.Context

	if server == "" {
		path, err := config.DefaultPath()
		if err != nil {
			return nil, err
		}
		cfg, err := config.Load(path)
		if err != nil {
			return nil, err
		}
		if contextName == "" {
			contextName = cfg.CurrentContext
		}
		var ctx *config.Context
		for i := range cfg.Contexts {
			if cfg.Contexts[i].Name == contextName {
				ctx = &cfg.Contexts[i]
				break
			}
		}
		if ctx == nil {
			return nil, errors.New("no server configured; run `novanasctl login` or pass --server")
		}
		server = ctx.Server
		if !insecure {
			insecure = ctx.InsecureSkipTLSVerify
		}
	}

	token := Globals.Token
	if token == "" && contextName != "" {
		if refresh, err := client.LoadRefreshToken(contextName); err == nil && refresh != "" {
			tmp := client.New(server, "", insecure)
			if tok, err := tmp.RefreshAccessToken(refresh); err == nil {
				token = tok.AccessToken
				if tok.RefreshToken != "" {
					_ = client.StoreRefreshToken(contextName, tok.RefreshToken)
				}
			}
		}
	}
	return client.New(server, token, insecure), nil
}

// emit formats v per the --output flag and writes it to stdout. tableFn is
// only invoked when output=table.
func emit(v any, tableFn func(io.Writer)) error {
	switch Globals.Output {
	case "json":
		return output.JSON(os.Stdout, v)
	case "yaml":
		return output.YAML(os.Stdout, v)
	case "table", "":
		if tableFn == nil {
			// fall back to JSON if no table renderer provided
			return output.JSON(os.Stdout, v)
		}
		tableFn(os.Stdout)
		return nil
	default:
		return fmt.Errorf("unknown output format %q", Globals.Output)
	}
}

// handleAPIError converts a 501 into the documented exit code / message.
func handleAPIError(err error) error {
	if errors.Is(err, client.ErrNotImplemented) {
		fmt.Fprintln(os.Stderr, "server not yet implemented")
		os.Exit(client.ExitNotImplemented)
	}
	return err
}

// decodeListItems coerces a raw JSON response into []map[string]any.
func decodeListItems(raw json.RawMessage) []map[string]any {
	var list client.ListResult
	if err := json.Unmarshal(raw, &list); err == nil && list.Items != nil {
		return list.Items
	}
	var arr []map[string]any
	if err := json.Unmarshal(raw, &arr); err == nil {
		return arr
	}
	return nil
}
