// Package secrets provides a small abstraction over secret storage with
// pluggable backends. The Manager interface is the only thing callers
// should depend on; concrete backends are selected at startup via
// FromEnv based on environment variables.
//
// Two backends are provided:
//
//   - FileBackend: stores secrets as files on local disk under a root
//     directory. Optionally encrypts contents at rest using AES-256-GCM
//     with a data-encryption-key (DEK) sealed by an external Sealer
//     (typically backed by the TPM, see internal/host/tpm).
//
//   - BaoBackend: reads/writes secrets in an OpenBao KV v2 mount over
//     HTTPS. We talk to the REST API directly rather than pulling in
//     the OpenBao Go client; KV v2 is a small, well-documented surface
//     and direct HTTP gives us tighter control over timeouts and error
//     handling. Secrets are stored under a single "value" field
//     base64-encoded, since KV v2 only persists string-valued fields.
//
// All backends are safe for concurrent use.
package secrets

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ErrNotFound is returned when a key does not exist.
var ErrNotFound = errors.New("secret not found")

// Manager is the secret-storage facade used by NovaNAS code that needs
// to read/write secrets at runtime. Both implementations are safe for
// concurrent use.
type Manager interface {
	// Get returns the raw bytes of a secret. Returns ErrNotFound if no
	// such key exists.
	Get(ctx context.Context, key string) ([]byte, error)

	// Set stores the secret. Overwrites if it exists.
	Set(ctx context.Context, key string, value []byte) error

	// Delete removes a secret. ErrNotFound if missing (callers usually
	// ignore this).
	Delete(ctx context.Context, key string) error

	// List returns keys with the given prefix, sorted lexicographically.
	List(ctx context.Context, prefix string) ([]string, error)

	// Backend returns a human-readable name for logs ("file", "bao").
	Backend() string
}

// validateKey enforces the shared key grammar: alphanumeric plus
// '/', '-', '_'. No leading or trailing slash, no ".." segment, no
// empty segments, no NUL bytes.
func validateKey(key string) error {
	if key == "" {
		return fmt.Errorf("secrets: empty key")
	}
	if strings.ContainsRune(key, 0) {
		return fmt.Errorf("secrets: NUL byte in key")
	}
	if strings.HasPrefix(key, "/") {
		return fmt.Errorf("secrets: leading slash not allowed: %q", key)
	}
	if strings.HasSuffix(key, "/") {
		return fmt.Errorf("secrets: trailing slash not allowed: %q", key)
	}
	for _, seg := range strings.Split(key, "/") {
		if seg == "" {
			return fmt.Errorf("secrets: empty path segment in key: %q", key)
		}
		if seg == ".." || seg == "." {
			return fmt.Errorf("secrets: path traversal not allowed: %q", key)
		}
		for _, r := range seg {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= 'A' && r <= 'Z':
			case r >= '0' && r <= '9':
			case r == '-' || r == '_':
			default:
				return fmt.Errorf("secrets: invalid character %q in key %q", r, key)
			}
		}
	}
	return nil
}

// validatePrefix is like validateKey but tolerates the empty string
// (meaning "list everything") and trailing slashes.
func validatePrefix(prefix string) error {
	if prefix == "" {
		return nil
	}
	if strings.ContainsRune(prefix, 0) {
		return fmt.Errorf("secrets: NUL byte in prefix")
	}
	if strings.HasPrefix(prefix, "/") {
		return fmt.Errorf("secrets: leading slash not allowed: %q", prefix)
	}
	trimmed := strings.TrimRight(prefix, "/")
	if trimmed == "" {
		return nil
	}
	for _, seg := range strings.Split(trimmed, "/") {
		if seg == "" {
			return fmt.Errorf("secrets: empty path segment in prefix: %q", prefix)
		}
		if seg == ".." || seg == "." {
			return fmt.Errorf("secrets: path traversal not allowed: %q", prefix)
		}
		for _, r := range seg {
			switch {
			case r >= 'a' && r <= 'z':
			case r >= 'A' && r <= 'Z':
			case r >= '0' && r <= '9':
			case r == '-' || r == '_':
			default:
				return fmt.Errorf("secrets: invalid character %q in prefix %q", r, prefix)
			}
		}
	}
	return nil
}
