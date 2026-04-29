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
// File format (identical to nova-bao-unseal):
//
//	2 bytes:  sealed-DEK length (uint16, big-endian)
//	N bytes:  TPM-sealed 32-byte AES-256 key
//	12 bytes: AES-GCM nonce
//	M bytes:  AES-GCM ciphertext+tag of the plaintext stash
package main

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/binary"
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

// wrap TPM-seals a fresh 32-byte AES-DEK and AES-GCM-encrypts plaintext
// with it. Mirrors cmd/nova-bao-unseal/main.go.
func wrap(plaintext []byte, sealer *tpm.Sealer) ([]byte, error) {
	dek := make([]byte, 32)
	if _, err := rand.Read(dek); err != nil {
		return nil, fmt.Errorf("rand: %w", err)
	}
	sealedDEK, err := sealer.Seal(dek)
	if err != nil {
		return nil, fmt.Errorf("tpm.Seal(DEK): %w", err)
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	ct := gcm.Seal(nil, nonce, plaintext, nil)
	out := make([]byte, 0, 2+len(sealedDEK)+len(nonce)+len(ct))
	hdr := make([]byte, 2)
	binary.BigEndian.PutUint16(hdr, uint16(len(sealedDEK)))
	out = append(out, hdr...)
	out = append(out, sealedDEK...)
	out = append(out, nonce...)
	out = append(out, ct...)
	return out, nil
}

func unwrap(blob []byte, sealer *tpm.Sealer) ([]byte, error) {
	if len(blob) < 2 {
		return nil, fmt.Errorf("blob too short")
	}
	dekLen := int(binary.BigEndian.Uint16(blob[:2]))
	if len(blob) < 2+dekLen+12 {
		return nil, fmt.Errorf("blob truncated")
	}
	sealedDEK := blob[2 : 2+dekLen]
	dek, err := sealer.Unseal(sealedDEK)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(dek)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := blob[2+dekLen : 2+dekLen+12]
	ct := blob[2+dekLen+12:]
	return gcm.Open(nil, nonce, ct, nil)
}

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

	sealed, err := wrap(plaintext, sealer)
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

	plaintext, err := unwrap(sealed, sealer)
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
