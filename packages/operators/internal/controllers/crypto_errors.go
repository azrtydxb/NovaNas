package controllers

import "errors"

// errNoKeyProvisioner is returned by encryption-aware reconcilers when no
// VolumeKeyProvisioner has been wired. Production builds MUST construct the
// operator with a real provisioner (TransitKeyProvisioner, or a shim around
// storage/internal/crypto.VolumeKeyManager); otherwise reconciliation of
// encrypted volumes is refused and surfaced as a condition on the CR.
//
// We deliberately do NOT fall back to NoopKeyProvisioner here: a deterministic
// placeholder wrapped blob would be written to status, the raw DK could never
// be recovered, and the volume's data would be permanently lost at mount time.
var errNoKeyProvisioner = errors.New(
	"no VolumeKeyProvisioner wired; refusing to reconcile encrypted resource",
)
