// Command nova-zfs-keyload is a boot-time helper that loads ZFS native-
// encryption keys for every dataset whose wrapped raw key has been
// escrowed in the secrets backend (typically OpenBao under
// nova/zfs-keys/<encoded-name>).
//
// Flow:
//
//  1. Connect to the secrets backend (env-driven; see secrets.FromEnv).
//  2. Open the TPM.
//  3. List secret keys under "zfs-keys/".
//  4. For each, fetch the wrapped record, TPM-unwrap, and feed `zfs
//     load-key -L prompt <name>` over stdin.
//
// Failure semantics:
//   - Missing zfs binary or TPM: hard exit non-zero (the consuming
//     services that Require= this unit will not start).
//   - Per-dataset failure (PCR mismatch, dataset gone, etc): logged
//     as ERROR but iteration continues. The exit code reflects whether
//     any dataset failed to load.
//
// The unit file is deploy/systemd/nova-zfs-keyload.service.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/novanas/nova-nas/internal/host/secrets"
	"github.com/novanas/nova-nas/internal/host/tpm"
	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

func main() {
	var (
		zfsBin   = flag.String("zfs", "/sbin/zfs", "Path to the zfs binary")
		logLevel = flag.String("log-level", "info", "Log level: debug, info, warn, error")
		timeout  = flag.Duration("timeout", 60*time.Second, "Per-dataset load-key timeout")
	)
	flag.Parse()

	logger := newLogger(*logLevel)

	if err := run(logger, *zfsBin, *timeout); err != nil {
		logger.Error("nova-zfs-keyload failed", "err", err)
		os.Exit(1)
	}
	logger.Info("nova-zfs-keyload: done")
}

func newLogger(level string) *slog.Logger {
	var lvl slog.Level
	switch strings.ToLower(level) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	return slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))
}

// run is the testable entry point. It is split from main so that
// tests can drive it with a fake secrets backend and a fake sealer
// without spawning an actual zfs binary.
func run(logger *slog.Logger, zfsBin string, perOpTimeout time.Duration) error {
	ctx := context.Background()

	sec, err := secrets.FromEnv(logger)
	if err != nil {
		return fmt.Errorf("secrets.FromEnv: %w", err)
	}

	sealer, err := tpm.New(logger)
	if err != nil {
		return fmt.Errorf("tpm.New: %w", err)
	}
	defer sealer.Close()

	mgr := &dataset.EncryptionManager{
		ZFSBin:  zfsBin,
		Sealer:  sealer,
		Secrets: sec,
	}

	return loadAll(ctx, logger, mgr, perOpTimeout)
}

// loadAll iterates every escrowed dataset and feeds its key to
// `zfs load-key`. Returns a non-nil error if at least one load failed,
// summarizing the failure count.
//
// Exposed so tests can drive it with a fake EncryptionManager.
func loadAll(ctx context.Context, logger *slog.Logger, mgr keyLoader, perOpTimeout time.Duration) error {
	datasets, err := mgr.ListEscrowedDatasets(ctx)
	if err != nil {
		return fmt.Errorf("list escrowed datasets: %w", err)
	}
	if len(datasets) == 0 {
		logger.Info("no escrowed datasets; nothing to do")
		return nil
	}
	logger.Info("loading escrowed dataset keys", "count", len(datasets))
	failed := 0
	for _, ds := range datasets {
		dctx, cancel := context.WithTimeout(ctx, perOpTimeout)
		err := mgr.LoadKey(dctx, ds)
		cancel()
		if err != nil {
			logger.Error("load-key failed", "dataset", ds, "err", err)
			failed++
			continue
		}
		logger.Info("load-key ok", "dataset", ds)
	}
	if failed > 0 {
		return fmt.Errorf("%d/%d datasets failed to load", failed, len(datasets))
	}
	return nil
}

// keyLoader is the minimal interface loadAll needs. *dataset.EncryptionManager
// satisfies it.
type keyLoader interface {
	ListEscrowedDatasets(ctx context.Context) ([]string, error)
	LoadKey(ctx context.Context, full string) error
}
