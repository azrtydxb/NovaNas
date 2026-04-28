package nvmeof

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// kernelStubHostDHChapAttrs creates the four DH-CHAP attribute files
// nvmet auto-creates when a host directory is mkdir'd. Tests against
// tmpfs must populate them manually before WriteFile is called.
func kernelStubHostDHChapAttrs(t *testing.T, root, hostNQN string) {
	t.Helper()
	base := filepath.Join(root, "nvmet/hosts", hostNQN)
	if err := os.MkdirAll(base, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, f := range []string{"dhchap_key", "dhchap_ctrl_key", "dhchap_hash", "dhchap_dhgroup"} {
		touch(t, filepath.Join(base, f))
	}
}

func TestSetHostDHChap_WritesFiles(t *testing.T) {
	m, root := newManager(t)
	hostNQN := "nqn.2024-01.io.novanas:client"
	kernelStubHostDHChapAttrs(t, root, hostNQN)

	cfg := DHChapConfig{
		Key:     "DHHC-1:01:" + strings.Repeat("a", 30),
		CtrlKey: "DHHC-1:01:" + strings.Repeat("b", 30),
		Hash:    "hmac(sha384)",
		DHGroup: "ffdhe2048",
	}
	if err := m.SetHostDHChap(context.Background(), hostNQN, cfg); err != nil {
		t.Fatalf("SetHostDHChap: %v", err)
	}

	base := filepath.Join(root, "nvmet/hosts", hostNQN)
	if got := readFile(t, filepath.Join(base, "dhchap_key")); got != cfg.Key {
		t.Errorf("dhchap_key=%q want %q", got, cfg.Key)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_ctrl_key")); got != cfg.CtrlKey {
		t.Errorf("dhchap_ctrl_key=%q want %q", got, cfg.CtrlKey)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_hash")); got != cfg.Hash {
		t.Errorf("dhchap_hash=%q want %q", got, cfg.Hash)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_dhgroup")); got != cfg.DHGroup {
		t.Errorf("dhchap_dhgroup=%q want %q", got, cfg.DHGroup)
	}
}

func TestSetHostDHChap_PartialUpdate(t *testing.T) {
	m, root := newManager(t)
	hostNQN := "nqn.2024-01.io.novanas:client2"
	kernelStubHostDHChapAttrs(t, root, hostNQN)

	// Pre-seed existing values.
	base := filepath.Join(root, "nvmet/hosts", hostNQN)
	if err := os.WriteFile(filepath.Join(base, "dhchap_hash"), []byte("hmac(sha256)"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(base, "dhchap_dhgroup"), []byte("null"), 0o644); err != nil {
		t.Fatal(err)
	}

	// Only set Key — hash/dhgroup must NOT be touched.
	cfg := DHChapConfig{Key: "DHHC-1:01:" + strings.Repeat("c", 30)}
	if err := m.SetHostDHChap(context.Background(), hostNQN, cfg); err != nil {
		t.Fatal(err)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_hash")); got != "hmac(sha256)" {
		t.Errorf("dhchap_hash unexpectedly changed to %q", got)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_dhgroup")); got != "null" {
		t.Errorf("dhchap_dhgroup unexpectedly changed to %q", got)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_key")); got != cfg.Key {
		t.Errorf("dhchap_key=%q", got)
	}
}

func TestSetHostDHChap_RejectsBadInput(t *testing.T) {
	m, _ := newManager(t)
	hostNQN := "nqn.2024-01.io.novanas:bad"

	cases := []struct {
		name string
		cfg  DHChapConfig
	}{
		{"bad_hash", DHChapConfig{Hash: "sha256"}},
		{"bad_dhgroup", DHChapConfig{DHGroup: "ffdhe1024"}},
		{"key_no_prefix", DHChapConfig{Key: strings.Repeat("a", 40)}},
		{"key_too_short", DHChapConfig{Key: "DHHC-1:01:abc"}},
		{"key_bad_chars", DHChapConfig{Key: "DHHC-1:01:" + strings.Repeat("a", 30) + " !"}},
		{"ctrl_key_bad", DHChapConfig{CtrlKey: "BOGUS:" + strings.Repeat("x", 40)}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := m.SetHostDHChap(context.Background(), hostNQN, tc.cfg); err == nil {
				t.Errorf("expected error for %s", tc.name)
			}
		})
	}
}

func TestClearHostDHChap_ResetsDefaults(t *testing.T) {
	m, root := newManager(t)
	hostNQN := "nqn.2024-01.io.novanas:clearme"
	kernelStubHostDHChapAttrs(t, root, hostNQN)

	// On real configfs each write replaces the value entirely. On tmpfs
	// the underlying file is opened O_WRONLY (no truncate) so partial
	// overwrites leave trailing bytes — which is fine for nvmet but
	// would confuse this test. Start from empty stubs.
	base := filepath.Join(root, "nvmet/hosts", hostNQN)
	for _, f := range []string{"dhchap_key", "dhchap_ctrl_key", "dhchap_hash", "dhchap_dhgroup"} {
		if err := os.WriteFile(filepath.Join(base, f), nil, 0o644); err != nil {
			t.Fatal(err)
		}
	}

	if err := m.ClearHostDHChap(context.Background(), hostNQN); err != nil {
		t.Fatalf("ClearHostDHChap: %v", err)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_key")); got != "" {
		t.Errorf("dhchap_key=%q want empty", got)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_ctrl_key")); got != "" {
		t.Errorf("dhchap_ctrl_key=%q want empty", got)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_hash")); got != "hmac(sha256)" {
		t.Errorf("dhchap_hash=%q", got)
	}
	if got := readFile(t, filepath.Join(base, "dhchap_dhgroup")); got != "null" {
		t.Errorf("dhchap_dhgroup=%q", got)
	}
}

func TestGetHostDHChap_ElidesSecrets(t *testing.T) {
	m, root := newManager(t)
	hostNQN := "nqn.2024-01.io.novanas:detail"
	kernelStubHostDHChapAttrs(t, root, hostNQN)

	base := filepath.Join(root, "nvmet/hosts", hostNQN)
	_ = os.WriteFile(filepath.Join(base, "dhchap_key"), []byte("DHHC-1:01:"+strings.Repeat("a", 30)), 0o644)
	_ = os.WriteFile(filepath.Join(base, "dhchap_ctrl_key"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(base, "dhchap_hash"), []byte("hmac(sha256)\n"), 0o644)
	_ = os.WriteFile(filepath.Join(base, "dhchap_dhgroup"), []byte("ffdhe2048\n"), 0o644)

	d, err := m.GetHostDHChap(context.Background(), hostNQN)
	if err != nil {
		t.Fatalf("GetHostDHChap: %v", err)
	}
	if !d.HasKey {
		t.Error("HasKey=false, want true")
	}
	if d.HasCtrlKey {
		t.Error("HasCtrlKey=true, want false")
	}
	if d.Hash != "hmac(sha256)" {
		t.Errorf("Hash=%q", d.Hash)
	}
	if d.DHGroup != "ffdhe2048" {
		t.Errorf("DHGroup=%q", d.DHGroup)
	}
}
