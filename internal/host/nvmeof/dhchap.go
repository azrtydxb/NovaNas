// Package nvmeof — DH-HMAC-CHAP in-band authentication (NVMe TP4022).
//
// This file extends the Manager with helpers to manage per-host DH-HMAC-CHAP
// configuration under nvmet/hosts/<hostnqn>/. The kernel exposes four
// attribute files per host:
//
//   - dhchap_key       — host secret in TP4022 form ("DHHC-1:NN:...:...:")
//   - dhchap_ctrl_key  — controller secret (bidirectional auth)
//   - dhchap_hash      — "hmac(sha256)" | "hmac(sha384)" | "hmac(sha512)"
//   - dhchap_dhgroup   — "null" | "ffdhe2048" | ... | "ffdhe8192"
//
// Keys are never returned by the read API — the Detail type only reports
// whether each secret is present.
package nvmeof

import (
	"context"
	"fmt"
	"path"
	"regexp"
	"strings"
)

// DHChapConfig is the per-host DH-HMAC-CHAP configuration.
type DHChapConfig struct {
	// Key is the host's secret in NVMe TP4022 format ("DHHC-1:NN:..."),
	// empty disables the host-side leg.
	Key string `json:"key,omitempty"`
	// CtrlKey is the controller-side secret for bidirectional auth,
	// empty means unidirectional (host → target only).
	CtrlKey string `json:"ctrlKey,omitempty"`
	// Hash algorithm: "hmac(sha256)" (default), "hmac(sha384)", "hmac(sha512)".
	Hash string `json:"hash,omitempty"`
	// DHGroup: "null" disables DH (raw HMAC); "ffdhe2048" through
	// "ffdhe8192" enable Diffie-Hellman key exchange.
	DHGroup string `json:"dhgroup,omitempty"`
}

// DHChapDetail mirrors DHChapConfig but elides secrets for display.
type DHChapDetail struct {
	HasKey     bool   `json:"hasKey"`
	HasCtrlKey bool   `json:"hasCtrlKey"`
	Hash       string `json:"hash,omitempty"`
	DHGroup    string `json:"dhgroup,omitempty"`
}

// validDHChapHashes is the set of accepted hash names. Empty string is
// also accepted by the validator and means "leave unchanged".
var validDHChapHashes = map[string]struct{}{
	"hmac(sha256)": {},
	"hmac(sha384)": {},
	"hmac(sha512)": {},
}

// validDHChapGroups is the set of accepted dhgroup names. Empty string
// is also accepted and means "leave unchanged".
var validDHChapGroups = map[string]struct{}{
	"null":      {},
	"ffdhe2048": {},
	"ffdhe3072": {},
	"ffdhe4096": {},
	"ffdhe6144": {},
	"ffdhe8192": {},
}

// dhchapKeyCharset restricts secrets to the alphabet legal in TP4022
// "DHHC-1:NN:base64:base64:" form.
var dhchapKeyCharset = regexp.MustCompile(`^[A-Za-z0-9+/=:.\-]+$`)

func validateDHChapKey(field, v string) error {
	if v == "" {
		return nil
	}
	if !strings.HasPrefix(v, "DHHC-1:") {
		return fmt.Errorf("%s: must start with %q", field, "DHHC-1:")
	}
	if len(v) < 32 || len(v) > 512 {
		return fmt.Errorf("%s: length %d outside [32,512]", field, len(v))
	}
	if !dhchapKeyCharset.MatchString(v) {
		return fmt.Errorf("%s: illegal characters", field)
	}
	return nil
}

func validateDHChapHash(v string) error {
	if v == "" {
		return nil
	}
	if _, ok := validDHChapHashes[v]; !ok {
		return fmt.Errorf("dhchap_hash: invalid value %q", v)
	}
	return nil
}

func validateDHChapGroup(v string) error {
	if v == "" {
		return nil
	}
	if _, ok := validDHChapGroups[v]; !ok {
		return fmt.Errorf("dhchap_dhgroup: invalid value %q", v)
	}
	return nil
}

func hostAttr(hostNQN, a string) string {
	return path.Join(hostDir(hostNQN), a)
}

// SetHostDHChap writes the non-empty fields of cfg to the host's nvmet
// attribute files. Empty fields are left untouched, allowing partial
// updates (e.g. rotate the host key without changing hash/group). To
// fully clear the configuration, use ClearHostDHChap.
func (m *Manager) SetHostDHChap(ctx context.Context, hostNQN string, cfg DHChapConfig) error {
	if err := validateNQN(hostNQN); err != nil {
		return err
	}
	if err := validateDHChapHash(cfg.Hash); err != nil {
		return err
	}
	if err := validateDHChapGroup(cfg.DHGroup); err != nil {
		return err
	}
	if err := validateDHChapKey("dhchap_key", cfg.Key); err != nil {
		return err
	}
	if err := validateDHChapKey("dhchap_ctrl_key", cfg.CtrlKey); err != nil {
		return err
	}
	if err := m.EnsureHost(ctx, hostNQN); err != nil {
		return err
	}
	c := m.cfs()
	if cfg.Key != "" {
		if err := c.WriteFile(hostAttr(hostNQN, "dhchap_key"), []byte(cfg.Key)); err != nil {
			return err
		}
	}
	if cfg.CtrlKey != "" {
		if err := c.WriteFile(hostAttr(hostNQN, "dhchap_ctrl_key"), []byte(cfg.CtrlKey)); err != nil {
			return err
		}
	}
	if cfg.Hash != "" {
		if err := c.WriteFile(hostAttr(hostNQN, "dhchap_hash"), []byte(cfg.Hash)); err != nil {
			return err
		}
	}
	if cfg.DHGroup != "" {
		if err := c.WriteFile(hostAttr(hostNQN, "dhchap_dhgroup"), []byte(cfg.DHGroup)); err != nil {
			return err
		}
	}
	return nil
}

// ClearHostDHChap erases both keys and resets hash/dhgroup to kernel
// defaults ("hmac(sha256)" / "null"). The host directory itself is
// preserved so any allowed_hosts symlinks pointing to it remain intact.
func (m *Manager) ClearHostDHChap(ctx context.Context, hostNQN string) error {
	if err := validateNQN(hostNQN); err != nil {
		return err
	}
	if err := m.EnsureHost(ctx, hostNQN); err != nil {
		return err
	}
	c := m.cfs()
	if err := c.WriteFile(hostAttr(hostNQN, "dhchap_key"), []byte("")); err != nil {
		return err
	}
	if err := c.WriteFile(hostAttr(hostNQN, "dhchap_ctrl_key"), []byte("")); err != nil {
		return err
	}
	if err := c.WriteFile(hostAttr(hostNQN, "dhchap_hash"), []byte("hmac(sha256)")); err != nil {
		return err
	}
	if err := c.WriteFile(hostAttr(hostNQN, "dhchap_dhgroup"), []byte("null")); err != nil {
		return err
	}
	return nil
}

// GetHostDHChap reports whether each secret is set and returns the
// hash/group as configured. Raw key material is intentionally never
// returned. If the host directory does not exist the error wraps
// configfs.ErrNotExist so callers can map to 404.
func (m *Manager) GetHostDHChap(_ context.Context, hostNQN string) (DHChapDetail, error) {
	if err := validateNQN(hostNQN); err != nil {
		return DHChapDetail{}, err
	}
	c := m.cfs()
	if _, err := c.ListDir(hostDir(hostNQN)); err != nil {
		return DHChapDetail{}, err
	}
	var d DHChapDetail
	if data, err := c.ReadFile(hostAttr(hostNQN, "dhchap_key")); err == nil {
		d.HasKey = strings.TrimSpace(string(data)) != ""
	}
	if data, err := c.ReadFile(hostAttr(hostNQN, "dhchap_ctrl_key")); err == nil {
		d.HasCtrlKey = strings.TrimSpace(string(data)) != ""
	}
	if data, err := c.ReadFile(hostAttr(hostNQN, "dhchap_hash")); err == nil {
		d.Hash = strings.TrimSpace(string(data))
	}
	if data, err := c.ReadFile(hostAttr(hostNQN, "dhchap_dhgroup")); err == nil {
		d.DHGroup = strings.TrimSpace(string(data))
	}
	return d, nil
}
