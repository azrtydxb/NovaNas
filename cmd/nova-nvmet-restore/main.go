// Command nova-nvmet-restore re-applies a saved NVMe-oF target
// configuration to the kernel's nvmet configfs at boot.
//
// It is intended to run as a oneshot systemd service after configfs is
// mounted and before nova-api starts. If the snapshot file is absent
// (first boot, or operator-cleared state), the binary logs an info
// message and exits 0; this is not an error condition.
//
// Configuration:
//
//	NOVA_NVMET_CONFIG  path to the snapshot JSON
//	                   (default: /etc/nova-nas/nvmet-config.json)
package main

import (
	"context"
	"errors"
	"log/slog"
	"os"

	"github.com/novanas/nova-nas/internal/host/configfs"
	"github.com/novanas/nova-nas/internal/host/nvmeof"
)

const defaultConfigPath = "/etc/nova-nas/nvmet-config.json"

func main() {
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{}))
	slog.SetDefault(logger)

	path := os.Getenv("NOVA_NVMET_CONFIG")
	if path == "" {
		path = defaultConfigPath
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		logger.Info("no saved nvmet config; nothing to restore", "path", path)
		os.Exit(0)
	} else if err != nil {
		logger.Error("stat config", "path", path, "err", err)
		os.Exit(1)
	}

	mgr := &nvmeof.Manager{CFS: &configfs.Manager{Root: configfs.DefaultRoot}}

	ctx := context.Background()
	if err := mgr.RestoreFromFile(ctx, path); err != nil {
		// errors.Is(err, os.ErrNotExist) is unreachable here because we
		// stat-checked above, but keep the branch for clarity if the
		// file disappears between stat and read (e.g. tmpfs race).
		if errors.Is(err, os.ErrNotExist) {
			logger.Info("config disappeared between stat and read; nothing to restore", "path", path)
			os.Exit(0)
		}
		logger.Error("restore failed", "path", path, "err", err)
		os.Exit(1)
	}
	logger.Info("nvmet configuration restored", "path", path)
}
