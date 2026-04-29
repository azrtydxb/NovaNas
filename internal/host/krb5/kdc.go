// Package krb5 — embedded MIT KDC server-side management.
//
// This file complements krb5.go (client-side krb5.conf/keytab/idmapd
// management). It manages the local KDC daemons (krb5kdc, kadmind) and
// the per-realm KDC database under /var/lib/krb5kdc.
//
// NovaNAS now ships an opt-in MIT KDC for service-principal issuance
// (machine-credential identity model). User principals are NOT minted
// here; Keycloak remains the source of truth for human users.
//
// All operations are local-DB through `kadmin.local` and `kdb5_util`,
// so no Kerberos auth is required. Tests inject an exec.Runner.
package krb5

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/novanas/nova-nas/internal/host/exec"
)

// DefaultRealm is the realm name used when the operator hasn't set one.
const DefaultRealm = "NOVANAS.LOCAL"

// KDCStatus is the observable runtime state of the embedded KDC.
type KDCStatus struct {
	Running        bool   `json:"running"`
	Realm          string `json:"realm"`
	DatabaseExists bool   `json:"databaseExists"`
	StashSealed    bool   `json:"stashSealed"`
	PrincipalCount int    `json:"principalCount"`
}

// KDCConfig is the input to KDCManager. All fields have sensible
// defaults so a zero-value KDCConfig works for the production layout.
type KDCConfig struct {
	// Realm is the KDC's realm name; defaults to DefaultRealm.
	Realm string
	// DatabasePath is the principal-DB file (kdb5_util writes
	// principal/principal.ok/principal.kadm5/principal.kadm5.lock here).
	DatabasePath string
	// StashPath is the runtime master-key stash. NovaNAS materializes
	// this file at boot from a TPM-sealed blob; see cmd/nova-kdc-unseal.
	// Defaults to /run/krb5kdc/.k5.<REALM> (tmpfs, mode 0600 root:root).
	StashPath string
	// SealedBlobPath is the on-disk TPM-sealed master-key blob produced
	// by `nova-kdc-unseal --init`. Existence of this file is the signal
	// reported as KDCStatus.StashSealed. Defaults to
	// /etc/nova-kdc/master.enc.
	SealedBlobPath string
	// KadminLocalBin overrides the kadmin.local binary path.
	KadminLocalBin string
	// Kdb5UtilBin overrides the kdb5_util binary path.
	Kdb5UtilBin string
	// SystemctlBin overrides the systemctl binary (used for status).
	SystemctlBin string
}

// KDCManager handles bootstrap, status, and lifecycle of the embedded KDC.
// The principal CRUD operations live on this type as well (see principal.go).
type KDCManager struct {
	Cfg    KDCConfig
	Runner exec.Runner
	FS     FileSystem
}

func (m *KDCManager) realm() string {
	if r := strings.TrimSpace(m.Cfg.Realm); r != "" {
		return r
	}
	return DefaultRealm
}

func (m *KDCManager) databasePath() string {
	if p := m.Cfg.DatabasePath; p != "" {
		return p
	}
	return "/var/lib/krb5kdc/principal"
}

func (m *KDCManager) stashPath() string {
	if p := m.Cfg.StashPath; p != "" {
		return p
	}
	// Runtime tmpfs path materialized by nova-kdc-unseal.service. The
	// MIT-default location (/var/lib/krb5kdc/.k5.<REALM>) is no longer
	// used at runtime — it only appears transiently during initial
	// bootstrap before being TPM-sealed and shredded.
	return "/run/krb5kdc/.k5." + m.realm()
}

func (m *KDCManager) sealedBlobPath() string {
	if p := m.Cfg.SealedBlobPath; p != "" {
		return p
	}
	return "/etc/nova-kdc/master.enc"
}

func (m *KDCManager) kadminLocalBin() string {
	if b := m.Cfg.KadminLocalBin; b != "" {
		return b
	}
	return "/usr/sbin/kadmin.local"
}

func (m *KDCManager) kdb5UtilBin() string {
	if b := m.Cfg.Kdb5UtilBin; b != "" {
		return b
	}
	return "/usr/sbin/kdb5_util"
}

func (m *KDCManager) systemctlBin() string {
	if b := m.Cfg.SystemctlBin; b != "" {
		return b
	}
	return "/bin/systemctl"
}

func (m *KDCManager) fs() FileSystem {
	if m.FS == nil {
		return osFS{}
	}
	return m.FS
}

func (m *KDCManager) run(ctx context.Context, bin string, args ...string) ([]byte, error) {
	runner := m.Runner
	if runner == nil {
		runner = exec.Run
	}
	return runner(ctx, bin, args...)
}

// Status returns the KDC's observable state. It does not start or stop
// services; it only reports.
func (m *KDCManager) Status(ctx context.Context) (*KDCStatus, error) {
	st := &KDCStatus{Realm: m.realm()}

	if _, err := m.fs().Stat(m.databasePath()); err == nil {
		st.DatabaseExists = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat %s: %w", m.databasePath(), err)
	}

	// StashSealed reports whether a TPM-sealed master-key blob exists
	// on disk. The runtime stash on tmpfs is intentionally NOT checked
	// here — its presence is a function of boot ordering, not of
	// long-term protection state.
	if _, err := m.fs().Stat(m.sealedBlobPath()); err == nil {
		st.StashSealed = true
	} else if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("stat %s: %w", m.sealedBlobPath(), err)
	}

	// `systemctl is-active krb5kdc` returns exit 0 with "active" on stdout
	// when running, non-zero otherwise. We tolerate the non-zero exit and
	// classify by stdout.
	out, _ := m.run(ctx, m.systemctlBin(), "is-active", "krb5kdc")
	if strings.TrimSpace(string(out)) == "active" {
		st.Running = true
	}

	// Best-effort principal count. If the DB doesn't exist or kadmin.local
	// fails, leave count at zero rather than failing the whole status call.
	if st.DatabaseExists {
		names, err := m.ListPrincipals(ctx)
		if err == nil {
			st.PrincipalCount = len(names)
		}
	}

	return st, nil
}

// errAlreadyBootstrapped is returned by Bootstrap when the KDC database
// already exists. Callers can decide whether that's a hard error or fine.
var errAlreadyBootstrapped = errors.New("krb5 kdc: database already exists")

// IsAlreadyBootstrapped reports whether err signals the KDC was already
// initialized.
func IsAlreadyBootstrapped(err error) bool { return errors.Is(err, errAlreadyBootstrapped) }

// Bootstrap creates the KDC database for the configured realm using a
// stash file (`kdb5_util create -s -r REALM -P <masterPassword>`). It is
// idempotent — if the database file already exists, it returns
// errAlreadyBootstrapped (use IsAlreadyBootstrapped to detect).
//
// masterPassword is only used at create time; afterwards the master key
// lives in the stash file.
//
// Threat model: the stash is a root:root 0600 file. TPM-sealing the
// master key is a documented follow-up (see docs/krb5/README.md).
func (m *KDCManager) Bootstrap(ctx context.Context, masterPassword string) error {
	if masterPassword == "" {
		return errors.New("krb5 kdc: master password required for bootstrap")
	}
	if _, err := m.fs().Stat(m.databasePath()); err == nil {
		return errAlreadyBootstrapped
	} else if !errors.Is(err, fs.ErrNotExist) {
		return fmt.Errorf("stat %s: %w", m.databasePath(), err)
	}
	args := []string{"-r", m.realm(), "-P", masterPassword, "create", "-s"}
	if _, err := m.run(ctx, m.kdb5UtilBin(), args...); err != nil {
		return fmt.Errorf("kdb5_util create: %w", err)
	}
	return nil
}
