package secrets

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
)

// Env var names. Documented on FromEnv.
const (
	envBackend = "SECRETS_BACKEND"

	envFileRoot    = "SECRETS_FILE_ROOT"
	envFileTPMSeal = "SECRETS_FILE_TPM_SEAL"

	envBaoAddr      = "BAO_ADDR"
	envVaultAddr    = "VAULT_ADDR"
	envBaoToken     = "BAO_TOKEN"
	envVaultToken   = "VAULT_TOKEN"
	envBaoTokenFile = "BAO_TOKEN_FILE"
	envBaoKVMount   = "BAO_KV_MOUNT"
	envBaoNamespace = "BAO_NAMESPACE"

	defaultFileRoot = "/etc/nova-nas/secrets"
	defaultKVMount  = "secret"
)

// tpmSealerFactory is the hook used by FromEnv to construct a Sealer
// when SECRETS_FILE_TPM_SEAL=true. It is package-level so that the
// tpm package (or tests) can install a real factory without creating
// an import cycle. If nil at the time FromEnv is called with TPM
// sealing enabled, FromEnv returns a clear configuration error.
//
// Wiring: cmd/host or whoever owns startup should do something like:
//
//	secrets.RegisterTPMSealerFactory(func() (secrets.Sealer, error) {
//	    return tpm.NewSealer()
//	})
//
// before calling secrets.FromEnv. This indirection lets the secrets
// package build cleanly without importing internal/host/tpm.
var tpmSealerFactory func() (Sealer, error)

// RegisterTPMSealerFactory installs the factory used to obtain a
// TPM-backed Sealer when SECRETS_FILE_TPM_SEAL=true. Calling this with
// nil clears the registration. Safe to call from main() at startup;
// not safe for concurrent use with FromEnv.
func RegisterTPMSealerFactory(f func() (Sealer, error)) {
	tpmSealerFactory = f
}

// FromEnv constructs a Manager from environment variables.
//
// SECRETS_BACKEND=file|bao (default: file)
//
// File backend env:
//
//	SECRETS_FILE_ROOT       default /etc/nova-nas/secrets
//	SECRETS_FILE_TPM_SEAL   "true" enables TPM sealing (uses the factory
//	                        registered via RegisterTPMSealerFactory,
//	                        normally backed by internal/host/tpm).
//	                        Default false on dev, recommended true in
//	                        production.
//
// Bao backend env:
//
//	BAO_ADDR (or VAULT_ADDR)    required
//	BAO_TOKEN (or VAULT_TOKEN)  required (or path via BAO_TOKEN_FILE)
//	BAO_KV_MOUNT                default "secret"
//	BAO_NAMESPACE               optional
//
// On any misconfiguration, returns a clear error.
func FromEnv(log *slog.Logger) (Manager, error) {
	if log == nil {
		log = slog.Default()
	}
	backend := strings.ToLower(strings.TrimSpace(os.Getenv(envBackend)))
	if backend == "" {
		backend = "file"
	}
	switch backend {
	case "file":
		return fileFromEnv(log)
	case "bao":
		return baoFromEnv(log)
	default:
		return nil, fmt.Errorf("secrets: unknown SECRETS_BACKEND %q (want file|bao)", backend)
	}
}

func fileFromEnv(log *slog.Logger) (Manager, error) {
	root := os.Getenv(envFileRoot)
	if root == "" {
		root = defaultFileRoot
	}
	var sealer Sealer
	if v := os.Getenv(envFileTPMSeal); v != "" {
		on, err := strconv.ParseBool(v)
		if err != nil {
			return nil, fmt.Errorf("secrets: invalid %s=%q: %w", envFileTPMSeal, v, err)
		}
		if on {
			if tpmSealerFactory == nil {
				return nil, fmt.Errorf("secrets: %s=true but no TPM sealer factory registered (call RegisterTPMSealerFactory at startup)", envFileTPMSeal)
			}
			s, err := tpmSealerFactory()
			if err != nil {
				return nil, fmt.Errorf("secrets: build TPM sealer: %w", err)
			}
			sealer = s
		}
	}
	return NewFileBackend(root, sealer, log)
}

func baoFromEnv(log *slog.Logger) (Manager, error) {
	addr := firstNonEmpty(os.Getenv(envBaoAddr), os.Getenv(envVaultAddr))
	if addr == "" {
		return nil, fmt.Errorf("secrets: bao backend requires %s or %s", envBaoAddr, envVaultAddr)
	}
	token := firstNonEmpty(os.Getenv(envBaoToken), os.Getenv(envVaultToken))
	if token == "" {
		if tf := os.Getenv(envBaoTokenFile); tf != "" {
			b, err := os.ReadFile(tf)
			if err != nil {
				return nil, fmt.Errorf("secrets: read %s=%q: %w", envBaoTokenFile, tf, err)
			}
			token = strings.TrimSpace(string(b))
		}
	}
	if token == "" {
		return nil, fmt.Errorf("secrets: bao backend requires %s, %s, or %s", envBaoToken, envVaultToken, envBaoTokenFile)
	}
	mount := os.Getenv(envBaoKVMount)
	if mount == "" {
		mount = defaultKVMount
	}
	return NewBaoBackend(BaoOpts{
		Address:   addr,
		Token:     token,
		KVMount:   mount,
		Namespace: os.Getenv(envBaoNamespace),
	}, log)
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}
