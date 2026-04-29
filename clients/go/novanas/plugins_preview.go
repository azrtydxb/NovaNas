package novanas

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// PluginProvisionedResource is one entry in PluginPermissions.WillCreate.
// It mirrors the server-side ProvisionedResource: a human-readable
// description of one resource the engine will create on Install.
type PluginProvisionedResource struct {
	Kind        string `json:"kind"`
	What        string `json:"what"`
	Destructive bool   `json:"destructive"`
}

// PluginPermissions is the structured "what will happen on Install"
// payload. Aurora's consent dialog renders this verbatim. The same
// payload is captured in the audit row when the user confirms, so the
// consent record is exact.
type PluginPermissions struct {
	WillCreate []PluginProvisionedResource `json:"willCreate"`
	WillMount  []string                    `json:"willMount"`
	WillOpen   []string                    `json:"willOpen"`
	Scopes     []string                    `json:"scopes"`
	Category   string                      `json:"category"`
}

// PluginPreview is the full GET /plugins/index/{name}/manifest response.
// Manifest is the parsed Plugin object (apiVersion/kind/metadata/spec)
// kept loosely-typed here so the SDK does not have to track every
// manifest schema bump.
type PluginPreview struct {
	Manifest      map[string]interface{} `json:"manifest"`
	Permissions   PluginPermissions      `json:"permissions"`
	TarballSHA256 string                 `json:"tarballSha256"`
}

// PreviewPlugin returns the manifest + structured permissions summary
// for a marketplace plugin. The server downloads + cosign-verifies the
// tarball before parsing the manifest, so a 422 response indicates a
// tampered package — surface that distinctly in callers' UIs.
//
// version is required (the marketplace publishes multiple per plugin
// and the consent record must pin one). Pass the version Aurora picked
// from the index entry.
func (c *Client) PreviewPlugin(ctx context.Context, name, version string) (*PluginPreview, error) {
	if name == "" {
		return nil, errors.New("novanas: name is required")
	}
	if version == "" {
		return nil, errors.New("novanas: version is required")
	}
	q := url.Values{}
	q.Set("version", version)
	var out PluginPreview
	if _, err := c.do(ctx, http.MethodGet, "/plugins/index/"+url.PathEscape(name)+"/manifest", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}
