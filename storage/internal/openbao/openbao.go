// Package openbao provides a thin client for the OpenBao (OSS HashiCorp
// Vault fork) Transit secrets engine. NovaNas uses Transit as the
// authoritative master-key holder: the Master Key never leaves Transit,
// and per-volume Dataset Keys are wrapped/unwrapped via the
// encrypt/decrypt endpoints.
//
// Two implementations live here:
//
//   - HTTPClient: the real HTTP client that talks to OpenBao over TLS.
//     Authenticates via a static token loaded from
//     --openbao-token-path (Kubernetes service-account mount).
//   - FakeTransit: an in-memory implementation for tests. Wrapping is
//     AES-256-GCM against a process-local master key. Sufficient for
//     correctness tests; emphatically not for production.
//
// References: docs/10-identity-and-secrets.md; OpenBao Transit API
// (https://openbao.org/api-docs/secret/transit/).
package openbao

import (
	"context"
	"time"
)

// TransitKeyConfig describes the master key's current state as reported
// by OpenBao. Only the subset NovaNas needs is modelled.
type TransitKeyConfig struct {
	Name          string
	Type          string // e.g. "aes256-gcm96"
	LatestVersion uint64
	MinVersion    uint64
	Exportable    bool
	CreatedAt     time.Time
}

// TransitClient is the minimal interface NovaNas uses against OpenBao
// Transit. Implementations must be safe for concurrent use.
type TransitClient interface {
	// WrapDK encrypts a raw 32-byte Dataset Key with the named master
	// key. Returns the opaque wrapped blob (a self-describing
	// ciphertext that encodes the master-key version) plus the master
	// key version used, so callers can record it in metadata.
	WrapDK(ctx context.Context, masterKeyName string, rawDK []byte) (wrapped []byte, version uint64, err error)

	// UnwrapDK reverses WrapDK. Because the master-key version is
	// baked into the wrapped blob, rotations of the master key do not
	// break previously-wrapped DKs.
	UnwrapDK(ctx context.Context, masterKeyName string, wrapped []byte) (rawDK []byte, err error)

	// RotateMasterKey asks OpenBao to generate a new version of the
	// named master key. New wraps use the new version automatically;
	// old wrapped blobs remain decryptable.
	RotateMasterKey(ctx context.Context, masterKeyName string) error

	// ReadConfig returns current key metadata.
	ReadConfig(ctx context.Context, masterKeyName string) (TransitKeyConfig, error)
}
