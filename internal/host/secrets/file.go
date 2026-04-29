package secrets

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
)

// dekFile is the on-disk filename of the sealed data-encryption-key
// inside Root. The leading dot keeps it out of List() results.
const dekFile = ".dek.sealed"

// dekSize is the AES-256 key size we generate.
const dekSize = 32

// gcmNonceSize is the standard 12-byte nonce for AES-GCM.
const gcmNonceSize = 12

// Sealer is the minimal interface FileBackend needs to seal/unseal a
// data-encryption-key. Satisfied by *internal/host/tpm.Sealer once that
// package lands; defined here so this package builds independently.
type Sealer interface {
	Seal(plaintext []byte) ([]byte, error)
	Unseal(sealed []byte) ([]byte, error)
}

// FileBackend stores secrets as files under Root, mode 0600.
//
// Optional Sealer: if non-nil, secrets are AES-GCM encrypted with a
// data-encryption-key (DEK) that is itself sealed via Sealer (e.g.
// TPM). The DEK is generated once at first write, sealed, and stored
// at <Root>/.dek.sealed. On reads, the DEK is unsealed once per
// Manager lifetime and cached in memory.
//
// Behavior when a Sealer is configured:
//
//   - On the first Set with no DEK on disk, a fresh 32-byte DEK is
//     generated, sealed via Sealer.Seal, written atomically to
//     <Root>/.dek.sealed, and cached. We chose lazy auto-create over
//     an explicit Init() call to keep the API small and to make the
//     "first boot writes its first secret" path work without a
//     bootstrap step. The cost is that read-only callers on a fresh
//     install will see ErrNotFound (correct) rather than a clearer
//     "uninitialized" error; in practice writes always precede reads.
//
//   - On any read, if the DEK is not yet cached, we attempt to load
//     <Root>/.dek.sealed and Unseal it. If the file is missing we
//     surface ErrNotFound for the user's key (it cannot exist without
//     a DEK) rather than treating that as a hard error.
type FileBackend struct {
	Root   string
	Sealer Sealer
	log    *slog.Logger

	mu     sync.Mutex
	dek    []byte // cached after first load/create; nil if no Sealer
	loaded bool   // true once we've attempted to load .dek.sealed
}

// NewFileBackend constructs a FileBackend rooted at root. The directory
// is created with mode 0700 if it does not exist. If sealer is nil,
// secrets are stored as plaintext files (mode 0600); if non-nil,
// secrets are AES-256-GCM encrypted with a DEK sealed by sealer.
func NewFileBackend(root string, sealer Sealer, log *slog.Logger) (*FileBackend, error) {
	if root == "" {
		return nil, fmt.Errorf("secrets: file backend root is empty")
	}
	if log == nil {
		log = slog.Default()
	}
	if err := os.MkdirAll(root, 0o700); err != nil {
		return nil, fmt.Errorf("secrets: mkdir %q: %w", root, err)
	}
	return &FileBackend{Root: root, Sealer: sealer, log: log}, nil
}

// Backend returns "file".
func (f *FileBackend) Backend() string { return "file" }

// pathFor resolves a validated key to its on-disk filesystem path.
func (f *FileBackend) pathFor(key string) (string, error) {
	if err := validateKey(key); err != nil {
		return "", err
	}
	return filepath.Join(f.Root, key), nil
}

// Get reads the secret at key. Returns ErrNotFound if the file does
// not exist or, when sealing is enabled, if the DEK file is missing.
func (f *FileBackend) Get(ctx context.Context, key string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	p, err := f.pathFor(key)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("secrets: read %q: %w", key, err)
	}
	if f.Sealer == nil {
		return data, nil
	}
	dek, err := f.loadDEK()
	if err != nil {
		return nil, err
	}
	if dek == nil {
		// No DEK on disk means no encrypted secrets can exist either.
		return nil, ErrNotFound
	}
	return decrypt(dek, data)
}

// Set writes the secret at key, creating parent directories as needed.
// Files are written atomically (write to temp + rename) with mode 0600.
func (f *FileBackend) Set(ctx context.Context, key string, value []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := f.pathFor(key)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		return fmt.Errorf("secrets: mkdir parent of %q: %w", key, err)
	}

	payload := value
	if f.Sealer != nil {
		dek, err := f.ensureDEK()
		if err != nil {
			return err
		}
		payload, err = encrypt(dek, value)
		if err != nil {
			return fmt.Errorf("secrets: encrypt %q: %w", key, err)
		}
	}
	return writeFileAtomic(p, payload, 0o600)
}

// Delete removes the secret at key. Returns ErrNotFound if absent.
func (f *FileBackend) Delete(ctx context.Context, key string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	p, err := f.pathFor(key)
	if err != nil {
		return err
	}
	if err := os.Remove(p); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return ErrNotFound
		}
		return fmt.Errorf("secrets: delete %q: %w", key, err)
	}
	// Best-effort cleanup of empty parent directories, up to but not
	// including Root.
	dir := filepath.Dir(p)
	for dir != f.Root && strings.HasPrefix(dir, f.Root) {
		if err := os.Remove(dir); err != nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

// List returns all keys under prefix, sorted. Files starting with "."
// (notably the sealed DEK) are skipped.
func (f *FileBackend) List(ctx context.Context, prefix string) ([]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := validatePrefix(prefix); err != nil {
		return nil, err
	}
	var keys []string
	walkErr := filepath.WalkDir(f.Root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if path == f.Root {
			return nil
		}
		name := d.Name()
		if strings.HasPrefix(name, ".") {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(f.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if prefix == "" || strings.HasPrefix(rel, prefix) {
			keys = append(keys, rel)
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("secrets: list %q: %w", prefix, walkErr)
	}
	sort.Strings(keys)
	return keys, nil
}

// loadDEK loads and caches the sealed DEK from disk. Returns (nil, nil)
// if no DEK file exists yet (i.e. nothing has been written).
func (f *FileBackend) loadDEK() ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.loaded {
		return f.dek, nil
	}
	sealed, err := os.ReadFile(filepath.Join(f.Root, dekFile))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			f.loaded = true
			return nil, nil
		}
		return nil, fmt.Errorf("secrets: read sealed DEK: %w", err)
	}
	dek, err := f.Sealer.Unseal(sealed)
	if err != nil {
		return nil, fmt.Errorf("secrets: unseal DEK: %w", err)
	}
	if len(dek) != dekSize {
		return nil, fmt.Errorf("secrets: unsealed DEK has wrong size %d", len(dek))
	}
	f.dek = dek
	f.loaded = true
	return f.dek, nil
}

// ensureDEK returns the DEK, creating and persisting a new one if none
// exists. Caller must have a non-nil Sealer.
func (f *FileBackend) ensureDEK() ([]byte, error) {
	dek, err := f.loadDEK()
	if err != nil {
		return nil, err
	}
	if dek != nil {
		return dek, nil
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	// Double-check after acquiring the write lock.
	if f.dek != nil {
		return f.dek, nil
	}
	fresh := make([]byte, dekSize)
	if _, err := rand.Read(fresh); err != nil {
		return nil, fmt.Errorf("secrets: generate DEK: %w", err)
	}
	sealed, err := f.Sealer.Seal(fresh)
	if err != nil {
		return nil, fmt.Errorf("secrets: seal DEK: %w", err)
	}
	if err := writeFileAtomic(filepath.Join(f.Root, dekFile), sealed, 0o600); err != nil {
		return nil, err
	}
	f.dek = fresh
	f.loaded = true
	f.log.Info("secrets: generated and sealed new DEK", "root", f.Root)
	return f.dek, nil
}

// writeFileAtomic writes data to path via a temp file in the same
// directory, then renames into place. mode is applied to the temp file
// before rename.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".tmp-*")
	if err != nil {
		return fmt.Errorf("secrets: create temp: %w", err)
	}
	tmpName := tmp.Name()
	defer func() {
		// If rename succeeded the temp file is gone; this is a no-op.
		_ = os.Remove(tmpName)
	}()
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secrets: chmod temp: %w", err)
	}
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("secrets: write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("secrets: close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		return fmt.Errorf("secrets: rename temp: %w", err)
	}
	return nil
}

// encrypt produces nonce(12) || ciphertext || tag(16) using AES-GCM.
func encrypt(key, plaintext []byte) ([]byte, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := make([]byte, gcmNonceSize)
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	// Seal appends ciphertext+tag to the (nonce) prefix.
	out := gcm.Seal(nonce, nonce, plaintext, nil)
	return out, nil
}

// decrypt expects nonce(12) || ciphertext || tag(16).
func decrypt(key, blob []byte) ([]byte, error) {
	if len(blob) < gcmNonceSize+16 {
		return nil, fmt.Errorf("secrets: ciphertext too short")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	nonce := blob[:gcmNonceSize]
	ct := blob[gcmNonceSize:]
	pt, err := gcm.Open(nil, nonce, ct, nil)
	if err != nil {
		return nil, fmt.Errorf("secrets: decrypt: %w", err)
	}
	return pt, nil
}
