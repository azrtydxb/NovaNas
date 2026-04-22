package cmd

import (
	"fmt"
	"os/exec"
	"runtime"

	"github.com/azrtydxb/novanas/packages/cli/internal/client"
	"github.com/azrtydxb/novanas/packages/cli/internal/config"
	"github.com/spf13/cobra"
)

func newLoginCmd() *cobra.Command {
	var (
		server      string
		contextName string
		insecure    bool
	)
	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in via OIDC device-code flow",
		RunE: func(cmd *cobra.Command, args []string) error {
			if server == "" {
				server = Globals.Server
			}
			if contextName == "" {
				contextName = Globals.Context
				if contextName == "" {
					contextName = "default"
				}
			}
			if server == "" {
				return fmt.Errorf("--server is required on first login")
			}

			c := client.New(server, "", insecure || Globals.InsecureSkipTLSVerify)
			dc, err := c.RequestDeviceCode()
			if err != nil {
				return err
			}

			uri := dc.VerificationURIComplete
			if uri == "" {
				uri = dc.VerificationURI
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Visit: %s\nCode:  %s\n", uri, dc.UserCode)
			openBrowser(uri)

			tok, err := c.PollToken(dc.DeviceCode, dc.Interval, dc.ExpiresIn)
			if err != nil {
				return err
			}

			path, err := config.DefaultPath()
			if err != nil {
				return err
			}
			cfg, err := config.Load(path)
			if err != nil {
				return err
			}
			cfg.Upsert(config.Context{
				Name:                  contextName,
				Server:                server,
				InsecureSkipTLSVerify: insecure,
			})
			if cfg.CurrentContext == "" {
				cfg.CurrentContext = contextName
			}
			if err := config.Save(path, cfg); err != nil {
				return err
			}
			if tok.RefreshToken != "" {
				if err := client.StoreRefreshToken(contextName, tok.RefreshToken); err != nil {
					fmt.Fprintf(cmd.ErrOrStderr(), "warning: could not store refresh token in keyring: %v\n", err)
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Logged in as context %q (%s)\n", contextName, server)
			return nil
		},
	}
	cmd.Flags().StringVar(&server, "login-server", "", "API server URL (first-time login)")
	cmd.Flags().StringVar(&contextName, "name", "", "context name to store this login under")
	cmd.Flags().BoolVar(&insecure, "skip-tls-verify", false, "skip TLS verification for this context")
	return cmd
}

func openBrowser(uri string) {
	if uri == "" {
		return
	}
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", uri)
	case "linux":
		cmd = exec.Command("xdg-open", uri)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", uri)
	default:
		return
	}
	_ = cmd.Start()
}
