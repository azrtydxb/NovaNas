package dataset

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/novanas/nova-nas/internal/host/exec"
	"github.com/novanas/nova-nas/internal/host/tpm"
	"github.com/novanas/nova-nas/internal/host/zfs/names"
)

// Default ZFS native-encryption algorithm used when CreateSpec
// requests encryption but does not specify one.
const DefaultEncryptionAlgorithm = "aes-256-gcm"

// RawKeyLen is the byte length of a ZFS raw encryption key. ZFS
// requires exactly 32 bytes for raw keys regardless of algorithm.
const RawKeyLen = 32

// SecretsManager is the minimal facade encryption code needs from
// the secrets package (mirrors internal/host/secrets.Manager so we
// avoid a hard dependency in this leaf-level package).
type SecretsManager interface {
	Get(ctx context.Context, key string) ([]byte, error)
	Set(ctx context.Context, key string, value []byte) error
	Delete(ctx context.Context, key string) error
	List(ctx context.Context, prefix string) ([]string, error)
}

// WrappedKeyRecord is the JSON envelope stored in OpenBao under
// nova/zfs-keys/<encoded-dataset-name>. The wrapped blob is opaque
// to the secrets backend — only nova-api and nova-zfs-keyload (which
// share the TPM) can unwrap it.
type WrappedKeyRecord struct {
	// Wrapped is the base64-encoded TPM envelope blob produced by
	// internal/host/tpm.WrapAEAD. Decoding yields a binary blob
	// suitable for tpm.UnwrapAEAD.
	Wrapped string `json:"wrapped"`
	// Algorithm is the ZFS encryption= property value (e.g. aes-256-gcm).
	Algorithm string `json:"algorithm"`
	// Created is the RFC3339 timestamp at which the wrapped key was
	// first written. Rotation overwrites this field.
	Created string `json:"created"`
}

// EncodeSecretKey turns a ZFS dataset full name (e.g. "tank/encrypted/v1")
// into the secrets key under which its wrapped raw key is stored.
//
// ZFS dataset names allow '.' and ':' which the secrets key grammar
// rejects (see internal/host/secrets.validateKey, which permits only
// [A-Za-z0-9-_/]). We escape such characters with a `__` prefix
// followed by two hex digits. The escape sequence is itself escaped
// (`_` → `__5F`) so encoding is unambiguously reversible.
//
// Slashes between dataset components are preserved as path separators
// in the secrets key. Examples:
//
//	tank/home              -> zfs-keys/tank/home
//	tank/host.example.com  -> zfs-keys/tank/host__2Eexample__2Ecom
//	tank/users/alice_42    -> zfs-keys/tank/users/alice__5F42
//	tank/legacy:set        -> zfs-keys/tank/legacy__3Aset
func EncodeSecretKey(datasetName string) (string, error) {
	if err := names.ValidateDatasetName(datasetName); err != nil {
		return "", err
	}
	parts := strings.Split(datasetName, "/")
	encParts := make([]string, len(parts))
	for i, p := range parts {
		encParts[i] = encodeSegment(p)
	}
	return "zfs-keys/" + strings.Join(encParts, "/"), nil
}

func encodeSegment(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-':
			b.WriteByte(c)
		default:
			// Includes '_' (must be escaped to keep encoding bijective),
			// '.', ':', and anything else that ValidateDatasetName let
			// through but the secrets-key grammar rejects.
			fmt.Fprintf(&b, "__%02X", c)
		}
	}
	return b.String()
}

// GenerateRawKey returns a fresh 32-byte ZFS raw key.
func GenerateRawKey() ([]byte, error) {
	k := make([]byte, RawKeyLen)
	if _, err := rand.Read(k); err != nil {
		return nil, fmt.Errorf("rand raw key: %w", err)
	}
	return k, nil
}

// EncryptionManager owns the lifecycle of TPM-sealed ZFS dataset keys.
// It glues together the TPM (for envelope encryption), the secrets
// backend (for storing wrapped blobs), and the zfs CLI (for create /
// load-key / unload-key with the unwrapped key fed via stdin).
type EncryptionManager struct {
	ZFSBin      string
	Sealer      tpm.SealUnsealer
	Secrets     SecretsManager
	StdinRunner exec.StdinRunner   // override for tests
	Now         func() time.Time   // override for tests; default time.Now
}

// runStdin returns the configured StdinRunner or exec.RunStdin.
func (m *EncryptionManager) runStdin() exec.StdinRunner {
	if m.StdinRunner != nil {
		return m.StdinRunner
	}
	return exec.RunStdin
}

func (m *EncryptionManager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now().UTC()
}

// CreateEncryptedArgs builds the argv for `zfs create` of an encrypted
// dataset. It does NOT include any reference to the key itself — the
// caller pipes the raw key over stdin.
//
//	zfs create -o encryption=<alg> -o keyformat=raw -o keylocation=prompt
//	           [-o k=v ...] [-V <size>] <full>
//
// keylocation=prompt makes ZFS read the raw key from stdin during
// create. After create, callers should `zfs set keylocation=prompt`
// has already been recorded; subsequent load-key calls also feed
// stdin.
func CreateEncryptedArgs(spec CreateSpec) ([]string, error) {
	if !spec.EncryptionEnabled {
		return nil, fmt.Errorf("CreateEncryptedArgs called on non-encrypted spec")
	}
	if spec.Type != "filesystem" && spec.Type != "volume" {
		return nil, fmt.Errorf("invalid dataset type %q", spec.Type)
	}
	full := spec.Parent + "/" + spec.Name
	if err := names.ValidateDatasetName(full); err != nil {
		return nil, err
	}
	alg := spec.EncryptionAlgorithm
	if alg == "" {
		alg = DefaultEncryptionAlgorithm
	}
	args := []string{"create",
		"-o", "encryption=" + alg,
		"-o", "keyformat=raw",
		"-o", "keylocation=prompt",
	}
	keys := make([]string, 0, len(spec.Properties))
	for k := range spec.Properties {
		// Reject conflicting overrides for the encryption-controlling
		// properties; the operator sets them via the dedicated fields.
		switch k {
		case "encryption", "keyformat", "keylocation":
			return nil, fmt.Errorf("property %q must not be set when EncryptionEnabled=true", k)
		}
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		args = append(args, "-o", k+"="+spec.Properties[k])
	}
	if spec.Type == "volume" {
		if spec.VolumeSizeBytes == 0 {
			return nil, fmt.Errorf("volume requires volumeSizeBytes")
		}
		args = append(args, "-V", fmt.Sprintf("%d", spec.VolumeSizeBytes))
	}
	args = append(args, full)
	return args, nil
}

// LoadKeyArgs builds argv for `zfs load-key -L prompt <name>`. The
// raw 32-byte key is fed via stdin.
func LoadKeyArgs(name string) ([]string, error) {
	if err := names.ValidateDatasetName(name); err != nil {
		return nil, err
	}
	return []string{"load-key", "-L", "prompt", name}, nil
}

// Initialize provisions a fresh raw key for a (not-yet-existing or
// existing-unkeyed) dataset, TPM-wraps it, persists the wrapped blob
// in the secrets backend, and then `zfs create`'s the dataset (when
// spec is non-nil) feeding the raw key over stdin.
//
// When spec is nil the dataset must already exist and this call only
// performs key escrow (e.g. recovery of metadata for an existing
// encrypted dataset). The unwrapped raw key is returned to the caller
// so they can drive zfs themselves.
func (m *EncryptionManager) Initialize(ctx context.Context, full string, spec *CreateSpec) ([]byte, error) {
	if err := names.ValidateDatasetName(full); err != nil {
		return nil, err
	}
	if m.Sealer == nil {
		return nil, fmt.Errorf("encryption manager: no TPM sealer configured")
	}
	if m.Secrets == nil {
		return nil, fmt.Errorf("encryption manager: no secrets backend configured")
	}
	rawKey, err := GenerateRawKey()
	if err != nil {
		return nil, err
	}
	wrapped, err := tpm.WrapAEAD(rawKey, m.Sealer)
	if err != nil {
		return nil, fmt.Errorf("tpm wrap: %w", err)
	}
	alg := DefaultEncryptionAlgorithm
	if spec != nil && spec.EncryptionAlgorithm != "" {
		alg = spec.EncryptionAlgorithm
	}
	rec := WrappedKeyRecord{
		Wrapped:   base64.StdEncoding.EncodeToString(wrapped),
		Algorithm: alg,
		Created:   m.now().Format(time.RFC3339),
	}
	body, err := json.Marshal(rec)
	if err != nil {
		return nil, fmt.Errorf("marshal record: %w", err)
	}
	secretKey, err := EncodeSecretKey(full)
	if err != nil {
		return nil, err
	}
	if err := m.Secrets.Set(ctx, secretKey, body); err != nil {
		return nil, fmt.Errorf("secrets set %s: %w", secretKey, err)
	}
	if spec != nil {
		args, err := CreateEncryptedArgs(*spec)
		if err != nil {
			// Roll back the escrow record so we don't leave a wrapped
			// key for a dataset that never came into existence.
			_ = m.Secrets.Delete(ctx, secretKey)
			return nil, err
		}
		if _, err := m.runStdin()(ctx, m.ZFSBin, rawKey, args...); err != nil {
			_ = m.Secrets.Delete(ctx, secretKey)
			return nil, fmt.Errorf("zfs create: %w", err)
		}
	}
	return rawKey, nil
}

// LoadKey fetches the wrapped blob from the secrets backend, TPM-
// unwraps it, and feeds the raw key to `zfs load-key` over stdin.
// Idempotent: if the dataset already has its key loaded, ZFS will
// return an error which the caller may choose to ignore — we surface
// it as-is.
func (m *EncryptionManager) LoadKey(ctx context.Context, full string) error {
	rawKey, err := m.fetchRawKey(ctx, full)
	if err != nil {
		return err
	}
	args, err := LoadKeyArgs(full)
	if err != nil {
		return err
	}
	if _, err := m.runStdin()(ctx, m.ZFSBin, rawKey, args...); err != nil {
		return fmt.Errorf("zfs load-key: %w", err)
	}
	return nil
}

// UnloadKey removes the in-memory key from ZFS. The wrapped blob
// remains in OpenBao so a subsequent LoadKey can re-mount the
// dataset.
func (m *EncryptionManager) UnloadKey(ctx context.Context, full string) error {
	if err := names.ValidateDatasetName(full); err != nil {
		return err
	}
	if _, err := m.runStdin()(ctx, m.ZFSBin, nil, "unload-key", full); err != nil {
		return fmt.Errorf("zfs unload-key: %w", err)
	}
	return nil
}

// Recover returns the raw 32-byte ZFS key for a dataset, decrypted
// in-process. Callers MUST treat this as the most-sensitive material
// possible: it is the only thing standing between an attacker and the
// dataset's plaintext. Audit logging is the caller's responsibility
// (the API handler that exposes Recover wraps this call with audit).
func (m *EncryptionManager) Recover(ctx context.Context, full string) ([]byte, error) {
	return m.fetchRawKey(ctx, full)
}

// HasKey reports whether the secrets backend holds a wrapped key for
// the given dataset. Used by listings to flag encrypted datasets in
// API responses.
func (m *EncryptionManager) HasKey(ctx context.Context, full string) (bool, error) {
	secretKey, err := EncodeSecretKey(full)
	if err != nil {
		return false, err
	}
	if _, err := m.Secrets.Get(ctx, secretKey); err != nil {
		// We do not import secrets.ErrNotFound here to avoid a
		// dependency cycle with the API layer. The handler is the
		// integration boundary that maps that sentinel.
		if strings.Contains(err.Error(), "not found") {
			return false, nil
		}
		return false, err
	}
	return true, nil
}

// ListEscrowedDatasets returns the full names of every dataset for
// which a wrapped key is stored. Used by nova-zfs-keyload at boot to
// iterate.
func (m *EncryptionManager) ListEscrowedDatasets(ctx context.Context) ([]string, error) {
	keys, err := m.Secrets.List(ctx, "zfs-keys/")
	if err != nil {
		return nil, fmt.Errorf("secrets list: %w", err)
	}
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		ds, err := DecodeSecretKey(k)
		if err != nil {
			// Skip malformed entries; surface via caller logging when
			// they hit decode at use-time.
			continue
		}
		out = append(out, ds)
	}
	sort.Strings(out)
	return out, nil
}

// DecodeSecretKey reverses EncodeSecretKey.
func DecodeSecretKey(k string) (string, error) {
	const prefix = "zfs-keys/"
	if !strings.HasPrefix(k, prefix) {
		return "", fmt.Errorf("not a zfs-keys/ secret: %q", k)
	}
	rest := strings.TrimPrefix(k, prefix)
	parts := strings.Split(rest, "/")
	out := make([]string, len(parts))
	for i, p := range parts {
		decoded, err := decodeSegment(p)
		if err != nil {
			return "", fmt.Errorf("decode segment %q: %w", p, err)
		}
		out[i] = decoded
	}
	full := strings.Join(out, "/")
	if err := names.ValidateDatasetName(full); err != nil {
		return "", fmt.Errorf("decoded name invalid: %w", err)
	}
	return full, nil
}

func decodeSegment(s string) (string, error) {
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '_' {
			// `__XX` (two underscores then two hex digits) is the
			// only legal use of '_' in an encoded segment.
			if i+4 > len(s) || s[i+1] != '_' || !isHex(s[i+2]) || !isHex(s[i+3]) {
				return "", fmt.Errorf("malformed escape at offset %d in %q", i, s)
			}
			var v byte
			for _, c := range []byte{s[i+2], s[i+3]} {
				v <<= 4
				switch {
				case c >= '0' && c <= '9':
					v |= c - '0'
				case c >= 'a' && c <= 'f':
					v |= c - 'a' + 10
				case c >= 'A' && c <= 'F':
					v |= c - 'A' + 10
				}
			}
			b.WriteByte(v)
			i += 3
			continue
		}
		b.WriteByte(s[i])
	}
	return b.String(), nil
}

func isHex(c byte) bool {
	return (c >= '0' && c <= '9') || (c >= 'a' && c <= 'f') || (c >= 'A' && c <= 'F')
}

func (m *EncryptionManager) fetchRawKey(ctx context.Context, full string) ([]byte, error) {
	if m.Sealer == nil {
		return nil, fmt.Errorf("encryption manager: no TPM sealer configured")
	}
	if m.Secrets == nil {
		return nil, fmt.Errorf("encryption manager: no secrets backend configured")
	}
	secretKey, err := EncodeSecretKey(full)
	if err != nil {
		return nil, err
	}
	body, err := m.Secrets.Get(ctx, secretKey)
	if err != nil {
		return nil, fmt.Errorf("secrets get %s: %w", secretKey, err)
	}
	var rec WrappedKeyRecord
	if err := json.Unmarshal(body, &rec); err != nil {
		return nil, fmt.Errorf("unmarshal record: %w", err)
	}
	wrapped, err := base64.StdEncoding.DecodeString(rec.Wrapped)
	if err != nil {
		return nil, fmt.Errorf("decode wrapped: %w", err)
	}
	rawKey, err := tpm.UnwrapAEAD(wrapped, m.Sealer)
	if err != nil {
		return nil, fmt.Errorf("tpm unwrap: %w", err)
	}
	if len(rawKey) != RawKeyLen {
		return nil, fmt.Errorf("recovered key wrong length: have %d, want %d", len(rawKey), RawKeyLen)
	}
	return rawKey, nil
}
