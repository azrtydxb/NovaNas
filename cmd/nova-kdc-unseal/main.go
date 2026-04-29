// nova-kdc-unseal mirrors nova-bao-unseal: it TPM-seals the MIT KDC
// master-key stash so the plaintext stash never has to live on
// persistent storage.
//
// Two modes:
//
//   --init    Read a plaintext stash file (typically the
//             /var/lib/krb5kdc/.k5.<REALM> produced by `kdb5_util create
//             -s`), TPM-seal it, and write the sealed blob to
//             --blob (default /etc/nova-kdc/master.enc, mode 0600).
//             After a successful init the operator MUST shred the
//             plaintext stash — this binary prints a warning to that
//             effect but does not delete the file itself.
//
//   default   Read the sealed blob, TPM-unseal, and write the plaintext
//             stash to --run-stash (default /run/krb5kdc/.k5.<REALM>,
//             mode 0600 root). Idempotent: if the run-stash already
//             exists with non-zero size we assume an earlier run
//             succeeded and exit 0. If the sealed blob does not exist
//             (operator opted out of TPM-sealing) we also exit 0 — the
//             KDC will fall back to whatever is configured for
//             key_stash_file in kdc.conf.
//
// Wire format (and wrap/unwrap implementation) is shared with
// nova-bao-unseal via internal/host/tpm.WrapAEAD / UnwrapAEAD.
package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/novanas/nova-nas/internal/host/tpm"
)

const (
	defaultRealm    = "NOVANAS.LOCAL"
	defaultBlobPath = "/etc/nova-kdc/master.enc"
	defaultRunDir   = "/run/krb5kdc"
)

func main() {
	var (
		initMode  = flag.Bool("init", false, "Initialize: TPM-seal the plaintext stash from --input and write to --blob")
		realm     = flag.String("realm", defaultRealm, "Kerberos realm name (used to derive default run-stash path)")
		blobPath  = flag.String("blob", defaultBlobPath, "Path to TPM-sealed master-key blob")
		runStash  = flag.String("run-stash", "", "Path to write plaintext stash on tmpfs (default /run/krb5kdc/.k5.<REALM>)")
		inputPath = flag.String("input", "", "(init only) Plaintext stash to read; '-' for stdin")
		logLevel  = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	)
	flag.Parse()

	var lvl slog.Level
	switch strings.ToLower(*logLevel) {
	case "debug":
		lvl = slog.LevelDebug
	case "warn":
		lvl = slog.LevelWarn
	case "error":
		lvl = slog.LevelError
	default:
		lvl = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: lvl}))

	stash := *runStash
	if stash == "" {
		stash = filepath.Join(defaultRunDir, ".k5."+*realm)
	}

	if *initMode {
		if err := runInit(logger, *blobPath, *inputPath); err != nil {
			logger.Error("init failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if err := runUnseal(logger, *blobPath, stash); err != nil {
		logger.Error("unseal failed", "err", err)
		os.Exit(1)
	}
}

// runInit reads the plaintext stash, TPM-seals it, and writes the blob.
// It deliberately does not delete the source file — the operator (or
// the bootstrap script) is responsible for shredding the plaintext.
func runInit(logger *slog.Logger, blobPath, inputPath string) error {
	logger.Info("nova-kdc-unseal: init mode", "blob", blobPath, "input", inputPath)

	if inputPath == "" {
		return errors.New("--input is required in --init mode (use '-' for stdin)")
	}

	var plaintext []byte
	if inputPath == "-" {
		var err error
		plaintext, err = io.ReadAll(os.Stdin)
		if err != nil {
			return fmt.Errorf("read stdin: %w", err)
		}
	} else {
		var err error
		plaintext, err = os.ReadFile(inputPath)
		if err != nil {
			return fmt.Errorf("read %s: %w", inputPath, err)
		}
	}
	if len(plaintext) == 0 {
		return errors.New("plaintext stash is empty")
	}

	sealer, err := tpm.New(logger)
	if err != nil {
		return fmt.Errorf("tpm.New: %w", err)
	}
	defer sealer.Close()

	sealed, err := tpm.WrapAEAD(plaintext, sealer)
	if err != nil {
		return fmt.Errorf("wrap: %w", err)
	}
	logger.Debug("wrapped (TPM-sealed DEK + AES-GCM)", "size_bytes", len(sealed))

	dir := filepath.Dir(blobPath)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := os.WriteFile(blobPath, sealed, 0o600); err != nil {
		return fmt.Errorf("write %s: %w", blobPath, err)
	}

	logger.Info("master-key stash sealed and written", "path", blobPath)
	if inputPath != "-" {
		logger.Warn("plaintext stash is still on disk; shred it now",
			"path", inputPath,
			"hint", "shred -u "+inputPath)
	}
	return nil
}

// runUnseal materializes the sealed blob into a tmpfs run-stash. It is
// idempotent: a non-empty run-stash short-circuits, and a missing blob
// is treated as "TPM-sealing not configured" and exits cleanly so the
// KDC can fall back to whatever key_stash_file kdc.conf points at.
func runUnseal(logger *slog.Logger, blobPath, runStash string) error {
	logger.Info("nova-kdc-unseal: unseal mode", "blob", blobPath, "run-stash", runStash)

	if fi, err := os.Stat(runStash); err == nil && fi.Size() > 0 {
		logger.Info("run-stash already present, no-op", "path", runStash, "size", fi.Size())
		return nil
	}

	sealed, err := os.ReadFile(blobPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("sealed blob not found, assuming operator opted out of TPM-sealing", "path", blobPath)
			return nil
		}
		return fmt.Errorf("read %s: %w", blobPath, err)
	}

	sealer, err := tpm.New(logger)
	if err != nil {
		return fmt.Errorf("tpm.New: %w", err)
	}
	defer sealer.Close()

	plaintext, err := tpm.UnwrapAEAD(sealed, sealer)
	if err != nil {
		if errors.Is(err, tpm.ErrPCRMismatch) {
			logger.Error("PCR mismatch: boot state may have changed since seal", "err", err)
			return fmt.Errorf("pcr mismatch: %w", err)
		}
		return fmt.Errorf("unwrap: %w", err)
	}
	logger.Debug("unwrapped", "size_bytes", len(plaintext))

	dir := filepath.Dir(runStash)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}
	if err := writeFile0600(runStash, plaintext); err != nil {
		return fmt.Errorf("write %s: %w", runStash, err)
	}

	logger.Info("master-key stash materialized to tmpfs", "path", runStash, "size", len(plaintext))
	return nil
}

// writeFile0600 writes data to path with mode 0600, replacing any
// existing file atomically (write to tmp + rename).
func writeFile0600(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	if err := os.Chmod(tmp, 0o600); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
