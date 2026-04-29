package dataset

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/novanas/nova-nas/internal/host/tpm"
)

// --- Fakes -----------------------------------------------------------------

// fakeSealer implements tpm.SealUnsealer with a stable XOR-style
// "encryption" so round-trips are deterministic.
type fakeSealer struct {
	failSeal   bool
	failUnseal bool
}

func (f *fakeSealer) Seal(plaintext []byte) ([]byte, error) {
	if f.failSeal {
		return nil, errors.New("fake seal failure")
	}
	out := make([]byte, len(plaintext)+1)
	out[0] = 0x42 // marker
	for i, b := range plaintext {
		out[i+1] = b ^ 0xA5
	}
	return out, nil
}

func (f *fakeSealer) Unseal(sealed []byte) ([]byte, error) {
	if f.failUnseal {
		return nil, tpm.ErrPCRMismatch
	}
	if len(sealed) < 1 || sealed[0] != 0x42 {
		return nil, errors.New("fake: bad sealed prefix")
	}
	out := make([]byte, len(sealed)-1)
	for i, b := range sealed[1:] {
		out[i] = b ^ 0xA5
	}
	return out, nil
}

// fakeSecrets is an in-memory SecretsManager.
type fakeSecrets struct {
	mu sync.Mutex
	m  map[string][]byte
}

func newFakeSecrets() *fakeSecrets {
	return &fakeSecrets{m: map[string][]byte{}}
}

func (f *fakeSecrets) Get(_ context.Context, k string) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	v, ok := f.m[k]
	if !ok {
		return nil, errors.New("secret not found")
	}
	cp := make([]byte, len(v))
	copy(cp, v)
	return cp, nil
}

func (f *fakeSecrets) Set(_ context.Context, k string, v []byte) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	cp := make([]byte, len(v))
	copy(cp, v)
	f.m[k] = cp
	return nil
}

func (f *fakeSecrets) Delete(_ context.Context, k string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.m, k)
	return nil
}

func (f *fakeSecrets) List(_ context.Context, prefix string) ([]string, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := []string{}
	for k := range f.m {
		if strings.HasPrefix(k, prefix) {
			out = append(out, k)
		}
	}
	return out, nil
}

// --- EncodeSecretKey -------------------------------------------------------

func TestEncodeSecretKey_RoundTrip(t *testing.T) {
	cases := []string{
		"tank/home",
		"tank/encrypted/v1",
		"tank/host.example.com/data",
		"tank/users/alice_42",
		"tank/legacy:set",
	}
	for _, ds := range cases {
		t.Run(ds, func(t *testing.T) {
			sk, err := EncodeSecretKey(ds)
			if err != nil {
				t.Fatalf("encode: %v", err)
			}
			if !strings.HasPrefix(sk, "zfs-keys/") {
				t.Errorf("missing prefix: %q", sk)
			}
			// Result must satisfy the secrets-key grammar
			// ([A-Za-z0-9-_/]) — no '.', ':', or '%'.
			for _, r := range sk {
				switch {
				case r >= 'a' && r <= 'z':
				case r >= 'A' && r <= 'Z':
				case r >= '0' && r <= '9':
				case r == '-' || r == '_' || r == '/':
				default:
					t.Errorf("illegal char %q in %q", r, sk)
				}
			}
			back, err := DecodeSecretKey(sk)
			if err != nil {
				t.Fatalf("decode: %v", err)
			}
			if back != ds {
				t.Errorf("round-trip: have %q, want %q", back, ds)
			}
		})
	}
}

func TestEncodeSecretKey_RejectsBadName(t *testing.T) {
	if _, err := EncodeSecretKey("bad@name"); err == nil {
		t.Error("expected error for snapshot-style name")
	}
}

// --- GenerateRawKey --------------------------------------------------------

func TestGenerateRawKey_Length(t *testing.T) {
	k, err := GenerateRawKey()
	if err != nil {
		t.Fatal(err)
	}
	if len(k) != RawKeyLen {
		t.Errorf("len=%d, want %d", len(k), RawKeyLen)
	}
	k2, _ := GenerateRawKey()
	if string(k) == string(k2) {
		t.Error("two consecutive generations produced identical keys")
	}
}

// --- CreateEncryptedArgs ---------------------------------------------------

func TestCreateEncryptedArgs_Filesystem(t *testing.T) {
	spec := CreateSpec{
		Parent:               "tank",
		Name:                 "secret",
		Type:                 "filesystem",
		EncryptionEnabled:    true,
		EncryptionAlgorithm:  "aes-256-gcm",
		Properties:           map[string]string{"compression": "lz4"},
	}
	args, err := CreateEncryptedArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, want := range []string{
		"create",
		"encryption=aes-256-gcm",
		"keyformat=raw",
		"keylocation=prompt",
		"compression=lz4",
		"tank/secret",
	} {
		if !strings.Contains(joined, want) {
			t.Errorf("missing %q in args: %v", want, args)
		}
	}
}

func TestCreateEncryptedArgs_DefaultsAlgorithm(t *testing.T) {
	spec := CreateSpec{
		Parent:            "tank",
		Name:              "secret",
		Type:              "filesystem",
		EncryptionEnabled: true,
	}
	args, err := CreateEncryptedArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(strings.Join(args, " "), "encryption="+DefaultEncryptionAlgorithm) {
		t.Errorf("expected default algorithm in args: %v", args)
	}
}

func TestCreateEncryptedArgs_RejectsConflictingProps(t *testing.T) {
	for _, bad := range []string{"encryption", "keyformat", "keylocation"} {
		spec := CreateSpec{
			Parent:            "tank",
			Name:              "secret",
			Type:              "filesystem",
			EncryptionEnabled: true,
			Properties:        map[string]string{bad: "x"},
		}
		if _, err := CreateEncryptedArgs(spec); err == nil {
			t.Errorf("expected rejection of %q in Properties", bad)
		}
	}
}

func TestCreateEncryptedArgs_Volume(t *testing.T) {
	spec := CreateSpec{
		Parent:            "tank",
		Name:              "vol",
		Type:              "volume",
		VolumeSizeBytes:   1 << 30,
		EncryptionEnabled: true,
	}
	args, err := CreateEncryptedArgs(spec)
	if err != nil {
		t.Fatal(err)
	}
	saw := false
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-V" && args[i+1] == "1073741824" {
			saw = true
		}
	}
	if !saw {
		t.Errorf("missing -V; args=%v", args)
	}
}

func TestCreateEncryptedArgs_RejectsNonEncryptedSpec(t *testing.T) {
	if _, err := CreateEncryptedArgs(CreateSpec{Parent: "tank", Name: "x", Type: "filesystem"}); err == nil {
		t.Error("expected error on non-encrypted spec")
	}
}

// buildCreateArgs must refuse encrypted specs (they go through the
// dedicated CreateEncryptedArgs path because zfs create needs stdin).
func TestBuildCreateArgs_RejectsEncryption(t *testing.T) {
	spec := CreateSpec{
		Parent:            "tank",
		Name:              "x",
		Type:              "filesystem",
		EncryptionEnabled: true,
	}
	if _, err := buildCreateArgs(spec); err == nil {
		t.Error("expected error: buildCreateArgs must not accept encrypted spec")
	}
}

// --- LoadKeyArgs -----------------------------------------------------------

func TestLoadKeyArgs(t *testing.T) {
	args, err := LoadKeyArgs("tank/encrypted")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"load-key", "-L", "prompt", "tank/encrypted"}
	if strings.Join(args, " ") != strings.Join(want, " ") {
		t.Errorf("have %v, want %v", args, want)
	}
}

// --- EncryptionManager round-trip -----------------------------------------

// recordingStdin captures invocations so we can assert that the raw
// key is in fact piped via stdin (and never appears in argv).
type recordingStdin struct {
	calls []recordedCall
}

type recordedCall struct {
	bin   string
	stdin []byte
	args  []string
}

func (r *recordingStdin) run(_ context.Context, bin string, stdin []byte, args ...string) ([]byte, error) {
	cp := make([]byte, len(stdin))
	copy(cp, stdin)
	r.calls = append(r.calls, recordedCall{bin: bin, stdin: cp, args: append([]string(nil), args...)})
	return nil, nil
}

func newTestEncMgr(rec *recordingStdin, sealer *fakeSealer, sec *fakeSecrets) *EncryptionManager {
	return &EncryptionManager{
		ZFSBin:      "/sbin/zfs",
		Sealer:      sealer,
		Secrets:     sec,
		StdinRunner: rec.run,
		Now:         func() time.Time { return time.Date(2026, 4, 29, 12, 0, 0, 0, time.UTC) },
	}
}

func TestEncryptionManager_InitializeRoundTrip(t *testing.T) {
	sealer := &fakeSealer{}
	sec := newFakeSecrets()
	rec := &recordingStdin{}
	m := newTestEncMgr(rec, sealer, sec)

	ctx := context.Background()
	spec := &CreateSpec{
		Parent:            "tank",
		Name:              "secret",
		Type:              "filesystem",
		EncryptionEnabled: true,
	}
	rawKey, err := m.Initialize(ctx, "tank/secret", spec)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if len(rawKey) != RawKeyLen {
		t.Fatalf("rawKey len=%d", len(rawKey))
	}

	// zfs create was called with the raw key on stdin and no key in argv.
	if len(rec.calls) != 1 {
		t.Fatalf("want 1 zfs call, got %d", len(rec.calls))
	}
	c := rec.calls[0]
	if string(c.stdin) != string(rawKey) {
		t.Error("zfs create did not receive the raw key on stdin")
	}
	for _, a := range c.args {
		if strings.Contains(a, base64.StdEncoding.EncodeToString(rawKey)) {
			t.Errorf("raw key appeared in argv: %q", a)
		}
	}

	// Secret was written, and its content unwraps back to the raw key.
	skey, _ := EncodeSecretKey("tank/secret")
	body, err := sec.Get(ctx, skey)
	if err != nil {
		t.Fatalf("secret missing: %v", err)
	}
	var stored WrappedKeyRecord
	if err := json.Unmarshal(body, &stored); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if stored.Algorithm != DefaultEncryptionAlgorithm {
		t.Errorf("stored algorithm=%q", stored.Algorithm)
	}
	if stored.Created != "2026-04-29T12:00:00Z" {
		t.Errorf("stored created=%q", stored.Created)
	}

	// Recover round-trip.
	got, err := m.Recover(ctx, "tank/secret")
	if err != nil {
		t.Fatalf("Recover: %v", err)
	}
	if string(got) != string(rawKey) {
		t.Error("Recover did not return the original raw key")
	}
}

func TestEncryptionManager_LoadKey_FeedsStdin(t *testing.T) {
	sealer := &fakeSealer{}
	sec := newFakeSecrets()
	rec := &recordingStdin{}
	m := newTestEncMgr(rec, sealer, sec)

	ctx := context.Background()
	rawKey, err := m.Initialize(ctx, "tank/secret", nil)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	rec.calls = nil // reset

	if err := m.LoadKey(ctx, "tank/secret"); err != nil {
		t.Fatalf("LoadKey: %v", err)
	}
	if len(rec.calls) != 1 {
		t.Fatalf("want 1 zfs call, got %d", len(rec.calls))
	}
	c := rec.calls[0]
	if string(c.stdin) != string(rawKey) {
		t.Error("zfs load-key did not receive the raw key on stdin")
	}
	if c.args[0] != "load-key" {
		t.Errorf("expected load-key, got %v", c.args)
	}
}

func TestEncryptionManager_LoadKey_PCRMismatchSurfaced(t *testing.T) {
	sealer := &fakeSealer{}
	sec := newFakeSecrets()
	rec := &recordingStdin{}
	m := newTestEncMgr(rec, sealer, sec)

	ctx := context.Background()
	if _, err := m.Initialize(ctx, "tank/secret", nil); err != nil {
		t.Fatalf("Initialize: %v", err)
	}

	// Now flip the sealer to fail unseal.
	sealer.failUnseal = true
	err := m.LoadKey(ctx, "tank/secret")
	if err == nil {
		t.Fatal("expected unseal error")
	}
	if !errors.Is(err, tpm.ErrPCRMismatch) {
		t.Errorf("expected ErrPCRMismatch in chain, got %v", err)
	}
}

func TestEncryptionManager_HasKey(t *testing.T) {
	sealer := &fakeSealer{}
	sec := newFakeSecrets()
	rec := &recordingStdin{}
	m := newTestEncMgr(rec, sealer, sec)

	ctx := context.Background()
	if has, err := m.HasKey(ctx, "tank/secret"); err != nil || has {
		t.Fatalf("HasKey before init: has=%v err=%v", has, err)
	}
	if _, err := m.Initialize(ctx, "tank/secret", nil); err != nil {
		t.Fatal(err)
	}
	if has, err := m.HasKey(ctx, "tank/secret"); err != nil || !has {
		t.Fatalf("HasKey after init: has=%v err=%v", has, err)
	}
}

func TestEncryptionManager_ListEscrowedDatasets(t *testing.T) {
	sealer := &fakeSealer{}
	sec := newFakeSecrets()
	rec := &recordingStdin{}
	m := newTestEncMgr(rec, sealer, sec)

	ctx := context.Background()
	for _, ds := range []string{"tank/a", "tank/b.x", "tank/c"} {
		if _, err := m.Initialize(ctx, ds, nil); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.ListEscrowedDatasets(ctx)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"tank/a", "tank/b.x", "tank/c"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Errorf("have %v, want %v", got, want)
	}
}

func TestEncryptionManager_InitializeRollsBackOnFailure(t *testing.T) {
	sealer := &fakeSealer{}
	sec := newFakeSecrets()
	rec := &recordingStdin{}

	// Make the zfs create runner fail.
	bad := func(_ context.Context, _ string, _ []byte, _ ...string) ([]byte, error) {
		return nil, errors.New("zfs create blew up")
	}
	m := &EncryptionManager{
		ZFSBin:      "/sbin/zfs",
		Sealer:      sealer,
		Secrets:     sec,
		StdinRunner: bad,
	}
	_ = rec

	ctx := context.Background()
	spec := &CreateSpec{
		Parent:            "tank",
		Name:              "secret",
		Type:              "filesystem",
		EncryptionEnabled: true,
	}
	if _, err := m.Initialize(ctx, "tank/secret", spec); err == nil {
		t.Fatal("expected failure")
	}
	skey, _ := EncodeSecretKey("tank/secret")
	if _, err := sec.Get(ctx, skey); err == nil {
		t.Error("escrow record was not rolled back after zfs failure")
	}
}
