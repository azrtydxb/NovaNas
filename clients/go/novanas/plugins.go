package novanas

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"time"
)

// PluginIndexVersion is one published version of a plugin in the
// marketplace index.
type PluginIndexVersion struct {
	Version      string    `json:"version"`
	TarballURL   string    `json:"tarballUrl"`
	SignatureURL string    `json:"signatureUrl"`
	SHA256       string    `json:"sha256,omitempty"`
	Size         int64     `json:"size,omitempty"`
	ReleasedAt   time.Time `json:"releasedAt,omitempty"`
}

// PluginIndexEntry is one plugin in the marketplace catalog.
type PluginIndexEntry struct {
	Name        string                `json:"name"`
	DisplayName string                `json:"displayName,omitempty"`
	Vendor      string                `json:"vendor"`
	Category    string                `json:"category"`
	Description string                `json:"description,omitempty"`
	Icon        string                `json:"icon,omitempty"`
	Homepage    string                `json:"homepage,omitempty"`
	Versions    []PluginIndexVersion `json:"versions"`
}

// PluginIndex is the GET /plugins/index response.
type PluginIndex struct {
	Version int                `json:"version"`
	Updated time.Time          `json:"updated,omitempty"`
	Plugins []PluginIndexEntry `json:"plugins"`
}

// PluginResource is one auto-provisioned `needs:` resource recorded
// against the plugin (used for purge-on-uninstall).
type PluginResource struct {
	Type string `json:"type"`
	ID   string `json:"id"`
}

// PluginInstallation is the GET /plugins/{name} response.
type PluginInstallation struct {
	ID          string                 `json:"id"`
	Name        string                 `json:"name"`
	Version     string                 `json:"version"`
	Manifest    map[string]interface{} `json:"manifest"`
	Status      string                 `json:"status"`
	InstalledAt time.Time              `json:"installedAt"`
	UpdatedAt   time.Time              `json:"updatedAt"`
	Resources   []PluginResource       `json:"resources,omitempty"`
}

// PluginInstallRequest is the body of POST /plugins.
type PluginInstallRequest struct {
	Name       string `json:"name"`
	Version    string `json:"version"`
	ValuesYAML string `json:"valuesYAML,omitempty"`
}

// PluginUpgradeRequest is the body of PATCH /plugins/{name}.
type PluginUpgradeRequest struct {
	Version string `json:"version"`
}

// ListPluginIndex returns the marketplace catalog. force=true bypasses
// the server's 15-minute cache.
func (c *Client) ListPluginIndex(ctx context.Context, force bool) (*PluginIndex, error) {
	q := url.Values{}
	if force {
		q.Set("refresh", "true")
	}
	var out PluginIndex
	if _, err := c.do(ctx, http.MethodGet, "/plugins/index", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPluginIndexEntry returns one entry from the marketplace catalog.
func (c *Client) GetPluginIndexEntry(ctx context.Context, name string) (*PluginIndexEntry, error) {
	if name == "" {
		return nil, errors.New("novanas: name is required")
	}
	var out PluginIndexEntry
	if _, err := c.do(ctx, http.MethodGet, "/plugins/index/"+url.PathEscape(name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ListPlugins returns all installed plugins.
func (c *Client) ListPlugins(ctx context.Context) ([]PluginInstallation, error) {
	var out []PluginInstallation
	if _, err := c.do(ctx, http.MethodGet, "/plugins", nil, nil, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// GetPlugin returns one installed plugin.
func (c *Client) GetPlugin(ctx context.Context, name string) (*PluginInstallation, error) {
	if name == "" {
		return nil, errors.New("novanas: name is required")
	}
	var out PluginInstallation
	if _, err := c.do(ctx, http.MethodGet, "/plugins/"+url.PathEscape(name), nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// InstallPlugin requests installation of a plugin from the marketplace.
func (c *Client) InstallPlugin(ctx context.Context, req PluginInstallRequest) (*PluginInstallation, error) {
	if req.Name == "" {
		return nil, errors.New("novanas: PluginInstallRequest.Name is required")
	}
	var out PluginInstallation
	if _, err := c.do(ctx, http.MethodPost, "/plugins", nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UpgradePlugin upgrades to req.Version.
func (c *Client) UpgradePlugin(ctx context.Context, name string, req PluginUpgradeRequest) (*PluginInstallation, error) {
	if name == "" {
		return nil, errors.New("novanas: name is required")
	}
	var out PluginInstallation
	if _, err := c.do(ctx, http.MethodPatch, "/plugins/"+url.PathEscape(name), nil, req, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UninstallPlugin removes an installed plugin. purge=true also unwinds
// the auto-provisioned `needs:` resources (datasets, oidcClients, …).
func (c *Client) UninstallPlugin(ctx context.Context, name string, purge bool) error {
	if name == "" {
		return errors.New("novanas: name is required")
	}
	q := url.Values{}
	if purge {
		q.Set("purge", "true")
	}
	_, err := c.do(ctx, http.MethodDelete, "/plugins/"+url.PathEscape(name), q, nil, nil)
	return err
}
