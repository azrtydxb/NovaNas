package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/novanas/nova-nas/internal/host/tpm"
)

// File format on disk: see internal/host/tpm.WrapAEAD/UnwrapAEAD. The
// implementation is shared with nova-kdc-unseal so the two binaries
// stay byte-compatible.

func main() {
	var (
		initMode   = flag.Bool("init", false, "Initialize: read plaintext unseal keys from stdin, encrypt via TPM, write to keys.enc")
		configPath = flag.String("config", "/etc/openbao/unseal/keys.enc", "Path to encrypted unseal keys file")
		baoAddr    = flag.String("addr", "https://127.0.0.1:8200", "OpenBao API address")
		maxRetries = flag.Int("max-retries", 5, "Maximum unseal attempts before giving up")
		retryDelay = flag.Duration("retry-delay", 2*time.Second, "Delay between unseal attempts")
		logLevel   = flag.String("log-level", "info", "Log level: debug, info, warn, error")
	)
	flag.Parse()

	// Configure logging
	var logLevelAtom slog.Level
	switch strings.ToLower(*logLevel) {
	case "debug":
		logLevelAtom = slog.LevelDebug
	case "warn":
		logLevelAtom = slog.LevelWarn
	case "error":
		logLevelAtom = slog.LevelError
	default:
		logLevelAtom = slog.LevelInfo
	}
	logger := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{
		Level: logLevelAtom,
	}))

	// Run appropriate mode
	if *initMode {
		if err := runInit(logger, *configPath); err != nil {
			logger.Error("init failed", "err", err)
			os.Exit(1)
		}
		return
	}

	if err := runUnseal(logger, *configPath, *baoAddr, *maxRetries, *retryDelay); err != nil {
		logger.Error("unseal failed", "err", err)
		os.Exit(1)
	}
}

// runInit reads plaintext unseal keys from stdin (JSON array of strings),
// encrypts them via TPM, and writes the encrypted blob to configPath.
func runInit(logger *slog.Logger, configPath string) error {
	logger.Info("nova-bao-unseal: init mode")

	// Initialize TPM sealer
	sealer, err := tpm.New(logger)
	if err != nil {
		return fmt.Errorf("tpm.New: %w", err)
	}
	defer sealer.Close()

	// Read plaintext unseal keys from stdin
	plaintext, err := io.ReadAll(os.Stdin)
	if err != nil {
		return fmt.Errorf("read stdin: %w", err)
	}
	plaintext = bytes.TrimSpace(plaintext)

	// Validate it's valid JSON array
	var keys []string
	if err := json.Unmarshal(plaintext, &keys); err != nil {
		return fmt.Errorf("parse unseal keys JSON: %w", err)
	}
	if len(keys) == 0 {
		return fmt.Errorf("no unseal keys provided")
	}
	logger.Info("read unseal keys", "count", len(keys))

	// Wrap: TPM-seal a fresh AES-DEK, AES-GCM encrypt plaintext with it.
	sealed, err := tpm.WrapAEAD(plaintext, sealer)
	if err != nil {
		return fmt.Errorf("wrap: %w", err)
	}
	logger.Debug("wrapped (TPM-sealed DEK + AES-GCM)", "size_bytes", len(sealed))

	// Ensure directory exists
	dir := filepath.Dir(configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("mkdir %s: %w", dir, err)
	}

	// Write encrypted blob
	if err := os.WriteFile(configPath, sealed, 0600); err != nil {
		return fmt.Errorf("write %s: %w", configPath, err)
	}

	logger.Info("unseal keys encrypted and written", "path", configPath)
	return nil
}

// runUnseal reads the encrypted unseal keys, decrypts them via TPM,
// and POSTs each to OpenBao's unseal endpoint.
func runUnseal(logger *slog.Logger, configPath, baoAddr string, maxRetries int, retryDelay time.Duration) error {
	logger.Info("nova-bao-unseal: unseal mode", "config", configPath, "addr", baoAddr)

	// Check if sealed file exists; if not, assume already unsealed or not initialized
	sealed, err := os.ReadFile(configPath)
	if err != nil {
		if os.IsNotExist(err) {
			logger.Info("unseal file not found, assuming already unsealed", "path", configPath)
			return nil
		}
		return fmt.Errorf("read %s: %w", configPath, err)
	}

	// Initialize TPM sealer
	sealer, err := tpm.New(logger)
	if err != nil {
		return fmt.Errorf("tpm.New: %w", err)
	}
	defer sealer.Close()

	// Unwrap: TPM-unseal the DEK, AES-GCM decrypt the payload.
	plaintext, err := tpm.UnwrapAEAD(sealed, sealer)
	if err != nil {
		if tpm.ErrPCRMismatch == err {
			logger.Error("PCR mismatch: boot state may have changed since seal", "err", err)
			return fmt.Errorf("pcr mismatch: %w", err)
		}
		return fmt.Errorf("unwrap: %w", err)
	}
	logger.Debug("unwrapped", "size_bytes", len(plaintext))

	// Parse unseal keys from plaintext
	var keys []string
	if err := json.Unmarshal(plaintext, &keys); err != nil {
		return fmt.Errorf("parse unsealed JSON: %w", err)
	}
	logger.Info("unsealed keys", "count", len(keys))

	// Create HTTP client that skips TLS verification for self-signed certs
	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// POST each unseal key to OpenBao
	for i, key := range keys {
		logger.Info("submitting unseal key", "index", i+1, "total", len(keys))

		payload := map[string]interface{}{"key": key}
		body, err := json.Marshal(payload)
		if err != nil {
			logger.Error("marshal unseal request", "index", i+1, "err", err)
			continue
		}

		// Retry loop for this key
		var lastErr error
		for attempt := 0; attempt < maxRetries; attempt++ {
			if attempt > 0 {
				time.Sleep(retryDelay)
			}

			req, err := http.NewRequest("POST", baoAddr+"/v1/sys/unseal", bytes.NewReader(body))
			if err != nil {
				logger.Error("new request", "index", i+1, "attempt", attempt+1, "err", err)
				lastErr = err
				continue
			}
			req.Header.Set("Content-Type", "application/json")

			resp, err := client.Do(req)
			if err != nil {
				logger.Warn("unseal request failed", "index", i+1, "attempt", attempt+1, "err", err)
				lastErr = err
				continue
			}

			respBody, _ := io.ReadAll(resp.Body)
			resp.Body.Close()

			// Success: 2xx status
			if resp.StatusCode >= 200 && resp.StatusCode < 300 {
				logger.Info("unseal key accepted", "index", i+1, "status", resp.StatusCode)
				lastErr = nil
				break
			}

			// Already unsealed is not an error for idempotency
			if resp.StatusCode == 400 {
				var errResp map[string]interface{}
				if err := json.Unmarshal(respBody, &errResp); err == nil {
					if errs, ok := errResp["errors"].([]interface{}); ok && len(errs) > 0 {
						if errMsg, ok := errs[0].(string); ok && strings.Contains(errMsg, "unsealed") {
							logger.Info("OpenBao already unsealed, no action needed", "index", i+1)
							return nil
						}
					}
				}
			}

			logger.Warn("unseal request failed", "index", i+1, "attempt", attempt+1, "status", resp.StatusCode, "body", string(respBody))
			lastErr = fmt.Errorf("unseal failed: %d %s", resp.StatusCode, string(respBody))
		}

		if lastErr != nil {
			logger.Error("unseal key rejected after retries", "index", i+1, "err", lastErr)
			// Continue with next key; the system may still unseal with remaining keys
		}
	}

	logger.Info("all unseal keys submitted")
	return nil
}
