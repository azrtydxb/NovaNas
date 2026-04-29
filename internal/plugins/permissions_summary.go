package plugins

import (
	"fmt"
	"strings"
)

// pluginAPIPrefix is where nova-api mounts every plugin's reverse-proxy
// routes. Kept in sync with internal/api/server.go's
// /plugins/{name}/api/* registration.
const pluginAPIPrefix = "/api/v1/plugins"

// ProvisionedResource describes one resource the engine will create
// when the plugin is installed. It is the user-facing translation of a
// `needs:` entry — short, human-readable, and explicit about whether
// it mutates non-plugin-owned state.
type ProvisionedResource struct {
	// Kind mirrors NeedKind ("dataset", "oidcClient", "tlsCert",
	// "permission"). Aurora keys icons + grouping off this.
	Kind string `json:"kind"`
	// What is a one-line description suitable for a consent bullet.
	What string `json:"what"`
	// Destructive is true when the resource mutates state the plugin
	// does NOT own (reuses an existing dataset, binds a global role,
	// etc.). For v1 every kind is non-destructive — engine creates
	// fresh resources scoped to the plugin — so this defaults to false.
	// The field exists so future manifest features (e.g. claiming an
	// existing dataset) can opt in.
	Destructive bool `json:"destructive"`
}

// PermissionsSummary is the structured view Aurora's "Install" dialog
// renders. It is derived purely from the manifest — no DB or marketplace
// state is consulted — and is also captured in the audit row when the
// user actually installs, so the consent record is exact.
type PermissionsSummary struct {
	// WillCreate is the list of auto-provisioned resources, one per
	// `needs:` entry, in declaration order.
	WillCreate []ProvisionedResource `json:"willCreate"`
	// WillMount lists the absolute nova-api paths the engine will
	// register on Install. One entry per spec.api.routes[*].path,
	// formatted as "{prefix}/api/v1/plugins/{name}{route.path}". The
	// trailing "/*" suffix denotes the catch-all subtree the route
	// proxies.
	WillMount []string `json:"willMount"`
	// WillOpen is reserved for future port-allocation declarations.
	// v1 manifests never declare ports so this is always [].
	WillOpen []string `json:"willOpen"`
	// Scopes is the set of nova-api permissions a caller needs to
	// install + later interact with this plugin. v1 surfaces only
	// the read scope; admin actions (install/uninstall/upgrade) are
	// gated by PermPluginsAdmin and not included here.
	Scopes []string `json:"scopes"`
	// Category mirrors spec.category — Aurora groups apps by it.
	Category string `json:"category"`
}

// Summarize derives a PermissionsSummary from a validated Plugin
// manifest. Pure function; no I/O. Safe to call on a partially-zero
// Plugin (returns empty slices, not nil) so JSON encodings stay
// consistent.
func Summarize(p *Plugin) PermissionsSummary {
	out := PermissionsSummary{
		WillCreate: []ProvisionedResource{},
		WillMount:  []string{},
		WillOpen:   []string{},
		Scopes:     []string{"PermPluginsRead"},
	}
	if p == nil {
		return out
	}
	out.Category = string(p.Spec.Category)

	for _, n := range p.Spec.Needs {
		out.WillCreate = append(out.WillCreate, summarizeNeed(p.Metadata.Name, n))
	}

	for _, r := range p.Spec.API.Routes {
		path := strings.TrimRight(r.Path, "/")
		if path == "" {
			path = "/"
		}
		// Mounted at /api/v1/plugins/{name}{route.path}/* — the
		// trailing /* makes it explicit to the operator that the
		// engine catches every subpath.
		mount := fmt.Sprintf("%s/%s%s/*", pluginAPIPrefix, p.Metadata.Name, path)
		out.WillMount = append(out.WillMount, mount)
	}

	return out
}

// summarizeNeed turns one Need into a one-line ProvisionedResource. The
// strings here are deliberately user-readable — Aurora displays them
// verbatim in the consent dialog.
func summarizeNeed(plugin string, n Need) ProvisionedResource {
	switch n.Kind {
	case NeedDataset:
		var what string
		if n.Dataset != nil && n.Dataset.Pool != "" && n.Dataset.Name != "" {
			what = fmt.Sprintf("ZFS dataset %s/%s", n.Dataset.Pool, n.Dataset.Name)
		} else {
			what = "ZFS dataset"
		}
		return ProvisionedResource{Kind: string(NeedDataset), What: what}
	case NeedOIDCClient:
		var what string
		if n.OIDCClient != nil && n.OIDCClient.ClientID != "" {
			what = fmt.Sprintf("Keycloak client %q", n.OIDCClient.ClientID)
		} else {
			what = "Keycloak client"
		}
		return ProvisionedResource{Kind: string(NeedOIDCClient), What: what}
	case NeedTLSCert:
		var what string
		if n.TLSCert != nil && n.TLSCert.CommonName != "" {
			what = fmt.Sprintf("TLS cert for %s", n.TLSCert.CommonName)
		} else {
			what = "TLS cert"
		}
		return ProvisionedResource{Kind: string(NeedTLSCert), What: what}
	case NeedPermission:
		var what string
		if n.Permission != nil && n.Permission.Role != "" {
			what = fmt.Sprintf("Bind realm role %q to plugin service account", n.Permission.Role)
		} else {
			what = "Bind realm role to plugin service account"
		}
		return ProvisionedResource{Kind: string(NeedPermission), What: what}
	default:
		return ProvisionedResource{Kind: string(n.Kind), What: fmt.Sprintf("Unknown need %q for plugin %q", n.Kind, plugin)}
	}
}
