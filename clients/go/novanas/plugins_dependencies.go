package novanas

import (
	"context"
	"errors"
	"net/http"
	"net/url"
)

// PluginDependency mirrors plugins.Dependency on the wire.
type PluginDependency struct {
	Name              string `json:"name"`
	VersionConstraint string `json:"versionConstraint,omitempty"`
	Source            string `json:"source"`
}

// PluginPlanStep mirrors plugins.PlanStep on the wire. It is one
// node of the resolver's install plan.
type PluginPlanStep struct {
	Name       string `json:"name"`
	Version    string `json:"version,omitempty"`
	Source     string `json:"source"`
	Action     string `json:"action"` // install | skip | bundled
	Constraint string `json:"constraint,omitempty"`
}

// PluginDependencyTreeNode mirrors plugins.DependencyTreeNode.
type PluginDependencyTreeNode struct {
	Name       string                     `json:"name"`
	Version    string                     `json:"version,omitempty"`
	Constraint string                     `json:"constraint,omitempty"`
	Source     string                     `json:"source"`
	Installed  bool                       `json:"installed"`
	Satisfied  bool                       `json:"satisfied"`
	Children   []PluginDependencyTreeNode `json:"children,omitempty"`
}

// PluginDependenciesResponse is the body of
// GET /plugins/{name}/dependencies.
type PluginDependenciesResponse struct {
	Tree *PluginDependencyTreeNode `json:"tree"`
	Plan []PluginPlanStep          `json:"plan"`
}

// PluginDependentsResponse is the body of
// GET /plugins/{name}/dependents.
type PluginDependentsResponse struct {
	Plugin     string   `json:"plugin"`
	Dependents []string `json:"dependents"`
}

// GetPluginDependencies returns the dependency graph for an installed
// plugin (or for an uninstalled one in the marketplace if version is
// supplied).
func (c *Client) GetPluginDependencies(ctx context.Context, name, version string) (*PluginDependenciesResponse, error) {
	if name == "" {
		return nil, errors.New("novanas: name is required")
	}
	q := url.Values{}
	if version != "" {
		q.Set("version", version)
	}
	var out PluginDependenciesResponse
	if _, err := c.do(ctx, http.MethodGet, "/plugins/"+url.PathEscape(name)+"/dependencies", q, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetPluginDependents returns the names of installed plugins whose
// manifests list `name` as a tier-2 dependency. Useful for "are you
// sure?" confirmation dialogs ahead of an uninstall.
func (c *Client) GetPluginDependents(ctx context.Context, name string) (*PluginDependentsResponse, error) {
	if name == "" {
		return nil, errors.New("novanas: name is required")
	}
	var out PluginDependentsResponse
	if _, err := c.do(ctx, http.MethodGet, "/plugins/"+url.PathEscape(name)+"/dependents", nil, nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// UninstallPluginForce is a convenience wrapper for
// DELETE /plugins/{name}?force=true. Without force the engine returns
// 409 when other installed plugins still depend on the target;
// callers can pass force=true to override (audit-logged server-side).
func (c *Client) UninstallPluginForce(ctx context.Context, name string, purge, force bool) error {
	if name == "" {
		return errors.New("novanas: name is required")
	}
	q := url.Values{}
	if purge {
		q.Set("purge", "true")
	}
	if force {
		q.Set("force", "true")
	}
	_, err := c.do(ctx, http.MethodDelete, "/plugins/"+url.PathEscape(name), q, nil, nil)
	return err
}
