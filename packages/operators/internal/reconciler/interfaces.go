package reconciler

import "context"

// NetworkClient abstracts host-level network state application (nmstate,
// systemd-networkd, etc.). Production implementations integrate with an
// external controller (e.g. kubernetes-nmstate); the default is a no-op.
type NetworkClient interface {
	// ApplyState submits a nmstate-style YAML state document for the named
	// node. Returns an opaque revision identifier when accepted.
	ApplyState(ctx context.Context, node string, stateYAML []byte) (revision string, err error)
	// ObservedState returns the last-observed nmstate YAML for the node, or
	// nil when unavailable.
	ObservedState(ctx context.Context, node string) ([]byte, error)
}

// NoopNetworkClient is used when no host-side networking operator is
// configured. ApplyState returns a deterministic revision so controllers
// can still write a non-empty status.
type NoopNetworkClient struct{}

// ApplyState returns a deterministic revision string.
func (NoopNetworkClient) ApplyState(_ context.Context, node string, _ []byte) (string, error) {
	return "noop-rev-" + node, nil
}

// ObservedState returns nil, nil.
func (NoopNetworkClient) ObservedState(_ context.Context, _ string) ([]byte, error) {
	return nil, nil
}

// UpdateClient abstracts OS-level update integration (RAUC, os-tree, etc.).
// Production implementations drive the on-host updater; tests inject
// NoopUpdateClient.
type UpdateClient interface {
	// CurrentVersion returns the currently-installed OS version.
	CurrentVersion(ctx context.Context) (string, error)
	// AvailableVersion returns the latest version advertised by the
	// configured channel, or "" when no update is available.
	AvailableVersion(ctx context.Context, channel string) (string, error)
	// Apply begins installation of the named version. Returns an opaque job
	// id that the caller records in status.
	Apply(ctx context.Context, version string) (jobID string, err error)
}

// NoopUpdateClient returns placeholder values suitable for dev / tests.
type NoopUpdateClient struct{}

// CurrentVersion returns "0.0.0-noop".
func (NoopUpdateClient) CurrentVersion(_ context.Context) (string, error) {
	return "0.0.0-noop", nil
}

// AvailableVersion returns "" (no update available).
func (NoopUpdateClient) AvailableVersion(_ context.Context, _ string) (string, error) {
	return "", nil
}

// Apply returns a deterministic placeholder job id.
func (NoopUpdateClient) Apply(_ context.Context, version string) (string, error) {
	return "noop-update-job-" + version, nil
}

