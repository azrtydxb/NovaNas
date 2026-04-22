package openbao

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"sync"
)

// FakeTransit is an in-memory TransitClient for tests. It models the
// master-key versioning behaviour of real OpenBao Transit: keys have
// numbered versions, rotate adds a new version, and unwrap works for
// any previously-emitted wrap.
//
// Wrapping uses AES-256-GCM under a random per-version master key.
// This is obviously not a replacement for real Transit — it exists
// solely to test the NovaNas integration without running an OpenBao
// server.
type FakeTransit struct {
	mu   sync.Mutex
	keys map[string]*fakeKey
}

type fakeKey struct {
	versions  map[uint64][]byte // version -> 32-byte master key
	latest    uint64
}

// NewFakeTransit constructs an empty fake.
func NewFakeTransit() *FakeTransit {
	return &FakeTransit{keys: make(map[string]*fakeKey)}
}

func (f *FakeTransit) get(name string) *fakeKey {
	k, ok := f.keys[name]
	if !ok {
		// Implicit create on first use, matching the typical operator
		// bootstrap flow where keys are created before use.
		k = &fakeKey{versions: map[uint64][]byte{}}
		f.keys[name] = k
		f.rotateLocked(k)
	}
	return k
}

func (f *FakeTransit) rotateLocked(k *fakeKey) {
	k.latest++
	mk := make([]byte, 32)
	_, _ = rand.Read(mk)
	k.versions[k.latest] = mk
}

// WrapDK wraps rawDK under the latest master key version.
func (f *FakeTransit) WrapDK(_ context.Context, masterKeyName string, rawDK []byte) ([]byte, uint64, error) {
	if len(rawDK) == 0 {
		return nil, 0, errors.New("fake-transit: empty plaintext")
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	k := f.get(masterKeyName)
	mk := k.versions[k.latest]

	block, err := aes.NewCipher(mk)
	if err != nil {
		return nil, 0, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, 0, err
	}
	nonce := make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, 0, err
	}
	ct := gcm.Seal(nil, nonce, rawDK, nil)

	// Blob format: "fakebao:v<N>:<base64(nonce||ciphertext)>".
	buf := make([]byte, 0, len(nonce)+len(ct))
	buf = append(buf, nonce...)
	buf = append(buf, ct...)
	wrapped := fmt.Sprintf("fakebao:v%d:%s", k.latest, base64.StdEncoding.EncodeToString(buf))
	return []byte(wrapped), k.latest, nil
}

// UnwrapDK reverses WrapDK, dispatching to the recorded master-key
// version embedded in the blob.
func (f *FakeTransit) UnwrapDK(_ context.Context, masterKeyName string, wrapped []byte) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k, ok := f.keys[masterKeyName]
	if !ok {
		return nil, fmt.Errorf("fake-transit: no such key %q", masterKeyName)
	}
	parts := strings.SplitN(string(wrapped), ":", 3)
	if len(parts) != 3 || parts[0] != "fakebao" || !strings.HasPrefix(parts[1], "v") {
		return nil, fmt.Errorf("fake-transit: bad wrapped blob")
	}
	var version uint64
	if _, err := fmt.Sscanf(parts[1], "v%d", &version); err != nil {
		return nil, fmt.Errorf("fake-transit: parse version: %w", err)
	}
	mk, ok := k.versions[version]
	if !ok {
		return nil, fmt.Errorf("fake-transit: missing master-key version %d", version)
	}
	raw, err := base64.StdEncoding.DecodeString(parts[2])
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(mk)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	ns := gcm.NonceSize()
	if len(raw) < ns {
		return nil, errors.New("fake-transit: truncated blob")
	}
	return gcm.Open(nil, raw[:ns], raw[ns:], nil)
}

// RotateMasterKey appends a new master-key version.
func (f *FakeTransit) RotateMasterKey(_ context.Context, masterKeyName string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := f.get(masterKeyName)
	f.rotateLocked(k)
	return nil
}

// ReadConfig reports key metadata.
func (f *FakeTransit) ReadConfig(_ context.Context, masterKeyName string) (TransitKeyConfig, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	k := f.get(masterKeyName)
	return TransitKeyConfig{
		Name:          masterKeyName,
		Type:          "aes256-gcm96",
		LatestVersion: k.latest,
		MinVersion:    1,
	}, nil
}

// touch is a silencer for the unused binary import when this file is
// compiled in isolation (keeps goimports happy if someone trims).
var _ = binary.BigEndian

// Compile-time check.
var _ TransitClient = (*FakeTransit)(nil)
