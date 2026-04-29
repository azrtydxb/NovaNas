package plugins

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"

	"github.com/novanas/nova-nas/internal/host/zfs/dataset"
)

// DatasetClient is the narrow surface DatasetProvisioner needs from the
// host ZFS dataset manager. Defined as an interface so tests can inject
// a fake without standing up a real `zfs` runner.
type DatasetClient interface {
	Get(ctx context.Context, name string) (*dataset.Detail, error)
	Create(ctx context.Context, spec dataset.CreateSpec) error
	Destroy(ctx context.Context, name string, recursive bool) error
}

// DatasetProvisioner fulfils plugins.DatasetNeed by creating ZFS
// datasets under the requested pool. It is idempotent: a Get probe
// short-circuits when the dataset already exists.
type DatasetProvisioner struct {
	Client DatasetClient
	Logger *slog.Logger
}

// resourceID encodes the full dataset path inside the synthetic ID so
// rollback can re-derive the name without an extra DB read.
//   format: "dataset:<plugin>/<full-name>" — full-name has '/' in it,
//   so we anchor on the first ":" and the first "/" after it to split
//   plugin from path.
func datasetResourceID(plugin, full string) string {
	return fmt.Sprintf("dataset:%s/%s", plugin, full)
}

// parseDatasetResourceID extracts the dataset full path. Tolerates the
// older NopProvisioner format ("dataset:<plugin>/<pool>/<name>") as a
// no-op since they coincide.
func parseDatasetResourceID(id string) (plugin, full string, ok bool) {
	if !strings.HasPrefix(id, "dataset:") {
		return "", "", false
	}
	rest := strings.TrimPrefix(id, "dataset:")
	slash := strings.IndexByte(rest, '/')
	if slash < 0 {
		return "", "", false
	}
	return rest[:slash], rest[slash+1:], true
}

// Provision creates pool/name with the requested properties. Returns
// the dataset's stable resource ID.
func (p *DatasetProvisioner) Provision(ctx context.Context, plugin string, n DatasetNeed) (string, error) {
	if p.Client == nil {
		return "", errors.New("plugins: DatasetProvisioner.Client is nil")
	}
	if n.Pool == "" || n.Name == "" {
		return "", fmt.Errorf("plugins: dataset need: pool+name required")
	}
	full := n.Pool + "/" + n.Name
	if _, err := p.Client.Get(ctx, full); err == nil {
		// Idempotent re-install.
		if p.Logger != nil {
			p.Logger.Info("plugins: dataset already exists; reusing", "plugin", plugin, "name", full)
		}
		return datasetResourceID(plugin, full), nil
	} else if !errors.Is(err, dataset.ErrNotFound) {
		return "", fmt.Errorf("plugins: dataset probe %q: %w", full, err)
	}
	spec := dataset.CreateSpec{
		Parent:     n.Pool,
		Name:       n.Name,
		Type:       "filesystem",
		Properties: n.Properties,
	}
	if err := p.Client.Create(ctx, spec); err != nil {
		return "", fmt.Errorf("plugins: dataset create %q: %w", full, err)
	}
	if p.Logger != nil {
		p.Logger.Info("plugins: dataset created", "plugin", plugin, "name", full)
	}
	return datasetResourceID(plugin, full), nil
}

// Unprovision destroys the dataset. recursive=false is intentional —
// if the plugin has carved out child datasets we refuse to delete them
// out from under it; the operator must clean up first.
func (p *DatasetProvisioner) Unprovision(ctx context.Context, plugin, resourceID string) error {
	if p.Client == nil {
		return errors.New("plugins: DatasetProvisioner.Client is nil")
	}
	_, full, ok := parseDatasetResourceID(resourceID)
	if !ok {
		return fmt.Errorf("plugins: bad dataset resource id %q", resourceID)
	}
	if _, err := p.Client.Get(ctx, full); errors.Is(err, dataset.ErrNotFound) {
		return nil
	}
	if err := p.Client.Destroy(ctx, full, false); err != nil {
		return fmt.Errorf("plugins: dataset destroy %q: %w", full, err)
	}
	if p.Logger != nil {
		p.Logger.Info("plugins: dataset destroyed", "plugin", plugin, "name", full)
	}
	return nil
}
