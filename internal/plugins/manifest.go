// Package plugins is the NovaNAS Tier 2 first-party plugin engine.
//
// Tier 2 plugins are NovaNAS-published, cosign-signed packages that
// integrate with the Aurora chrome (extend the nova-api API namespace
// + ship a React UI window). They are distinct from Tier 1 (the core
// OS) and Tier 3 (community Helm charts under /api/v1/workloads).
//
// The package is organized as:
//
//   - manifest.go    — the Plugin resource (YAML) + validator
//   - marketplace.go — index/tarball/signature client
//   - verify.go      — cosign verifier
//   - needs.go       — auto-provisioner for dataset/oidcClient/tlsCert/permission
//   - router.go      — runtime API-route mount/unmount + reverse-proxy
//   - ui_assets.go   — UI bundle unpack + static serving
//   - lifecycle.go   — install/uninstall/upgrade orchestration + DB persistence
//   - store.go       — DB DAO (hand-written; sqlc replacement for v1)
package plugins

import (
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Masterminds/semver/v3"
	"gopkg.in/yaml.v3"
)

// CurrentAPIVersion is the manifest schema version this engine speaks.
const CurrentAPIVersion = "novanas.io/v1"

// Kind is the only supported manifest kind.
const Kind = "Plugin"

// Category gates what kinds of `needs:` a plugin may claim. The
// validator rejects manifests that ask for privileged resources
// outside their declared category.
type Category string

const (
	CategoryStorage      Category = "storage"
	CategoryNetworking   Category = "networking"
	CategoryObservability Category = "observability"
	CategoryDeveloper    Category = "developer"
	CategoryUtility      Category = "utility"
)

var validCategories = map[Category]bool{
	CategoryStorage:       true,
	CategoryNetworking:    true,
	CategoryObservability: true,
	CategoryDeveloper:     true,
	CategoryUtility:       true,
}

// DisplayCategory is the user-facing App Center grouping. Orthogonal to
// the privilege-axis Category which controls which `needs:` kinds a
// plugin may claim. The engine ignores DisplayCategory for privilege
// decisions — it exists purely so Aurora can group/filter installed
// and marketplace plugins by intent.
type DisplayCategory string

const (
	DisplayBackup        DisplayCategory = "backup"
	DisplayFiles         DisplayCategory = "files"
	DisplayMultimedia    DisplayCategory = "multimedia"
	DisplayPhotos        DisplayCategory = "photos"
	DisplayProductivity  DisplayCategory = "productivity"
	DisplaySecurity      DisplayCategory = "security"
	DisplayCommunication DisplayCategory = "communication"
	DisplayHome          DisplayCategory = "home"
	DisplayDeveloper     DisplayCategory = "developer"
	DisplayNetwork       DisplayCategory = "network"
	DisplayStorage       DisplayCategory = "storage"
	DisplaySurveillance  DisplayCategory = "surveillance"
	DisplayUtilities     DisplayCategory = "utilities"
	DisplayObservability DisplayCategory = "observability"
)

// AllDisplayCategories is the canonical, ordered list of valid display
// categories. Aurora's App Center sidebar renders these in this order
// regardless of which plugins are installed.
var AllDisplayCategories = []DisplayCategory{
	DisplayBackup,
	DisplayFiles,
	DisplayMultimedia,
	DisplayPhotos,
	DisplayProductivity,
	DisplaySecurity,
	DisplayCommunication,
	DisplayHome,
	DisplayDeveloper,
	DisplayNetwork,
	DisplayStorage,
	DisplaySurveillance,
	DisplayUtilities,
	DisplayObservability,
}

var validDisplayCategories = func() map[DisplayCategory]bool {
	m := make(map[DisplayCategory]bool, len(AllDisplayCategories))
	for _, c := range AllDisplayCategories {
		m[c] = true
	}
	return m
}()

// IsValidDisplayCategory reports whether c is one of the 14 known
// display categories.
func IsValidDisplayCategory(c DisplayCategory) bool {
	return validDisplayCategories[c]
}

// DisplayCategoryDisplayName returns the human-friendly name Aurora
// renders in the sidebar.
func DisplayCategoryDisplayName(c DisplayCategory) string {
	switch c {
	case DisplayBackup:
		return "Backup"
	case DisplayFiles:
		return "Files"
	case DisplayMultimedia:
		return "Multimedia"
	case DisplayPhotos:
		return "Photos"
	case DisplayProductivity:
		return "Productivity"
	case DisplaySecurity:
		return "Security"
	case DisplayCommunication:
		return "Communication"
	case DisplayHome:
		return "Home"
	case DisplayDeveloper:
		return "Developer"
	case DisplayNetwork:
		return "Network"
	case DisplayStorage:
		return "Storage"
	case DisplaySurveillance:
		return "Surveillance"
	case DisplayUtilities:
		return "Utilities"
	case DisplayObservability:
		return "Observability"
	}
	return string(c)
}

// DefaultDisplayCategoryFor returns the display-axis category we infer
// when a plugin author omits `displayCategory`. Only the privilege
// categories with an obvious 1:1 match map; anything else is left empty
// and Aurora groups those under "Other".
func DefaultDisplayCategoryFor(privCategory Category) DisplayCategory {
	switch privCategory {
	case CategoryStorage:
		return DisplayStorage
	case CategoryNetworking:
		return DisplayNetwork
	case CategoryObservability:
		return DisplayObservability
	case CategoryDeveloper:
		return DisplayDeveloper
	case CategoryUtility:
		return DisplayUtilities
	}
	return ""
}

// tagRE constrains a single plugin tag. Lowercase alphanumerics with
// optional dashes; must start with alphanumeric.
var tagRE = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// MaxTagLength is the per-tag character cap.
const MaxTagLength = 32

// MaxTags is the per-plugin tag-count cap. Prevents abuse of the index
// (large/unbounded tag arrays inflate the merged catalog response).
const MaxTags = 16

// DeploymentType is the runtime substrate the plugin runs on.
type DeploymentType string

const (
	DeploymentHelm    DeploymentType = "helm"
	DeploymentSystemd DeploymentType = "systemd"
)

// AuthMode controls how nova-api forwards authentication on a
// reverse-proxied API route.
type AuthMode string

const (
	// AuthBearerPassthrough forwards the caller's Keycloak JWT to the
	// upstream verbatim. The upstream MUST verify it itself.
	AuthBearerPassthrough AuthMode = "bearer-passthrough"
	// AuthServiceToken strips the caller's auth and mints a fresh
	// service token using the plugin's own oidcClient credentials.
	AuthServiceToken AuthMode = "service-token"
)

// NeedKind is the type of resource the plugin asks the engine to
// auto-provision at install time.
type NeedKind string

const (
	NeedDataset    NeedKind = "dataset"
	NeedOIDCClient NeedKind = "oidcClient"
	NeedTLSCert    NeedKind = "tlsCert"
	NeedPermission NeedKind = "permission"
)

// Plugin is the top-level manifest resource. It is the contract every
// Tier 2 plugin author writes against.
type Plugin struct {
	APIVersion string         `yaml:"apiVersion" json:"apiVersion"`
	Kind       string         `yaml:"kind" json:"kind"`
	Metadata   PluginMetadata `yaml:"metadata" json:"metadata"`
	Spec       PluginSpec     `yaml:"spec" json:"spec"`
}

// PluginMetadata identifies the plugin and pins what was signed.
//
// Signature is informational — the actual cryptographic verification
// happens against the .sig artifact in the marketplace tarball — but
// we record it here so installations can be audited.
type PluginMetadata struct {
	Name      string `yaml:"name" json:"name"`
	Version   string `yaml:"version" json:"version"`
	Vendor    string `yaml:"vendor" json:"vendor"`
	Signature string `yaml:"signature,omitempty" json:"signature,omitempty"`
}

// PluginSpec is the bulk of the manifest.
type PluginSpec struct {
	Description     string          `yaml:"description" json:"description"`
	Category        Category        `yaml:"category" json:"category"`
	DisplayCategory DisplayCategory `yaml:"displayCategory,omitempty" json:"displayCategory,omitempty"`
	Tags            []string        `yaml:"tags,omitempty" json:"tags,omitempty"`
	Icon            string          `yaml:"icon,omitempty" json:"icon,omitempty"`
	Deployment   Deployment   `yaml:"deployment" json:"deployment"`
	Needs        []Need       `yaml:"needs,omitempty" json:"needs,omitempty"`
	Dependencies []Dependency `yaml:"dependencies,omitempty" json:"dependencies,omitempty"`
	API          APISpec      `yaml:"api,omitempty" json:"api,omitempty"`
	UI           UISpec       `yaml:"ui,omitempty" json:"ui,omitempty"`
	Health       Health       `yaml:"health,omitempty" json:"health,omitempty"`
	Lifecycle    Lifecycle    `yaml:"lifecycle,omitempty" json:"lifecycle,omitempty"`
}

// DependencySource enumerates where the engine should look up a
// dependency. `tier-2` deps come from a registered marketplace and
// trigger a recursive install. `bundled` deps are documentation-only
// — they describe a NovaNAS core feature the plugin relies on (e.g.
// "ZFS replication") and are satisfied implicitly.
type DependencySource string

const (
	DependencySourceTier2   DependencySource = "tier-2"
	DependencySourceBundled DependencySource = "bundled"
)

var validDependencySources = map[DependencySource]bool{
	DependencySourceTier2:   true,
	DependencySourceBundled: true,
}

// Dependency declares a prerequisite plugin that must be installed
// (and at a satisfying version) before this plugin's own provisioning
// runs.
type Dependency struct {
	Name              string           `yaml:"name" json:"name"`
	VersionConstraint string           `yaml:"versionConstraint,omitempty" json:"versionConstraint,omitempty"`
	Source            DependencySource `yaml:"source" json:"source"`
}

// Deployment is how the plugin's runtime is started.
//
// Helm: a chart bundled inside the package tarball at chart/.
// Systemd: a unit file at systemd/<name>.service.
type Deployment struct {
	Type      DeploymentType `yaml:"type" json:"type"`
	Chart     string         `yaml:"chart,omitempty" json:"chart,omitempty"`         // helm: relative path inside tarball
	Namespace string         `yaml:"namespace,omitempty" json:"namespace,omitempty"` // helm
	Unit      string         `yaml:"unit,omitempty" json:"unit,omitempty"`           // systemd: unit name
}

// Need declares an auto-provisioned resource. Exactly one of the
// kind-specific fields must be populated for the kind.
type Need struct {
	Kind NeedKind `yaml:"kind" json:"kind"`
	// Dataset
	Dataset *DatasetNeed `yaml:"dataset,omitempty" json:"dataset,omitempty"`
	// OIDCClient
	OIDCClient *OIDCClientNeed `yaml:"oidcClient,omitempty" json:"oidcClient,omitempty"`
	// TLSCert
	TLSCert *TLSCertNeed `yaml:"tlsCert,omitempty" json:"tlsCert,omitempty"`
	// Permission
	Permission *PermissionNeed `yaml:"permission,omitempty" json:"permission,omitempty"`
}

type DatasetNeed struct {
	Pool       string            `yaml:"pool" json:"pool"`
	Name       string            `yaml:"name" json:"name"`
	Properties map[string]string `yaml:"properties,omitempty" json:"properties,omitempty"`
}

type OIDCClientNeed struct {
	ClientID  string   `yaml:"clientId" json:"clientId"`
	Redirects []string `yaml:"redirectUris,omitempty" json:"redirectUris,omitempty"`
	Public    bool     `yaml:"public,omitempty" json:"public,omitempty"`
}

type TLSCertNeed struct {
	CommonName string   `yaml:"commonName" json:"commonName"`
	DNSNames   []string `yaml:"dnsNames,omitempty" json:"dnsNames,omitempty"`
	IPs        []string `yaml:"ips,omitempty" json:"ips,omitempty"`
	TTLDays    int      `yaml:"ttlDays,omitempty" json:"ttlDays,omitempty"`
}

type PermissionNeed struct {
	Role        string `yaml:"role" json:"role"`
	Description string `yaml:"description,omitempty" json:"description,omitempty"`
}

// APISpec lists the upstream routes nova-api will reverse-proxy under
// /api/v1/plugins/{name}/api/.
type APISpec struct {
	Routes []APIRoute `yaml:"routes,omitempty" json:"routes,omitempty"`
}

type APIRoute struct {
	Path     string   `yaml:"path" json:"path"`         // e.g. "/buckets" — mounted at /api/v1/plugins/{name}/api/buckets
	Upstream string   `yaml:"upstream" json:"upstream"` // base URL the plugin's process serves on
	Scopes   []string `yaml:"scopes,omitempty" json:"scopes,omitempty"`
	Auth     AuthMode `yaml:"auth" json:"auth"`
}

// UISpec describes the React window Aurora opens for this plugin.
type UISpec struct {
	Window UIWindow `yaml:"window,omitempty" json:"window,omitempty"`
}

type UIWindow struct {
	Name   string `yaml:"name" json:"name"`     // human title
	Icon   string `yaml:"icon,omitempty" json:"icon,omitempty"`
	Route  string `yaml:"route" json:"route"`   // Aurora route, e.g. "/apps/rustfs"
	Bundle string `yaml:"bundle" json:"bundle"` // entry file in ui/, e.g. "main.js"
}

// Health describes how nova-api probes the plugin process. Optional —
// Aurora uses these to colour the running indicator on the UI window.
type Health struct {
	Path           string `yaml:"path,omitempty" json:"path,omitempty"`
	IntervalSeconds int    `yaml:"intervalSeconds,omitempty" json:"intervalSeconds,omitempty"`
	TimeoutSeconds  int    `yaml:"timeoutSeconds,omitempty" json:"timeoutSeconds,omitempty"`
}

// Lifecycle hooks run by the engine at the named transitions. Each
// value is an executable path inside the package tarball
// (relative to its root).
type Lifecycle struct {
	PreInstall   string `yaml:"preInstall,omitempty" json:"preInstall,omitempty"`
	PostInstall  string `yaml:"postInstall,omitempty" json:"postInstall,omitempty"`
	PreUninstall string `yaml:"preUninstall,omitempty" json:"preUninstall,omitempty"`
}

// nameRE enforces DNS-1123 label rules on plugin names. The name
// becomes part of URLs, k8s namespace, and on-disk path so the
// strictest reasonable form is right.
var nameRE = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,38}[a-z0-9])?$`)

// versionRE accepts SemVer (with optional v prefix). We don't lean on
// blang/semver here to keep the dep surface small — the marketplace
// produces well-formed versions.
var versionRE = regexp.MustCompile(`^v?[0-9]+\.[0-9]+\.[0-9]+(-[0-9A-Za-z.-]+)?(\+[0-9A-Za-z.-]+)?$`)

// privilegedNeedsByCategory enumerates which need kinds a category may
// claim. Categories not listed here may only claim `permission`. This
// is the privilege-escalation guard called out in the spec.
var privilegedNeedsByCategory = map[Category]map[NeedKind]bool{
	CategoryStorage: {
		NeedDataset: true, NeedOIDCClient: true, NeedTLSCert: true, NeedPermission: true,
	},
	CategoryNetworking: {
		NeedOIDCClient: true, NeedTLSCert: true, NeedPermission: true,
	},
	CategoryObservability: {
		NeedOIDCClient: true, NeedTLSCert: true, NeedPermission: true,
	},
	CategoryDeveloper: {
		NeedOIDCClient: true, NeedPermission: true,
	},
	CategoryUtility: {
		NeedPermission: true,
	},
}

// ParseManifest decodes YAML bytes into a Plugin and runs validation.
func ParseManifest(data []byte) (*Plugin, error) {
	var p Plugin
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&p); err != nil {
		return nil, fmt.Errorf("plugins: yaml: %w", err)
	}
	// Fill in displayCategory from the privilege category when the
	// author omitted it. Categories without a 1:1 mapping stay empty
	// (Aurora groups those under "Other"). The fill-in runs before
	// Validate so the empty-OK rule below is consistent.
	if p.Spec.DisplayCategory == "" {
		p.Spec.DisplayCategory = DefaultDisplayCategoryFor(p.Spec.Category)
	}
	if err := p.Validate(); err != nil {
		return nil, err
	}
	return &p, nil
}

// Validate runs strict structural + privilege checks on the manifest.
// All errors are joined so authors see every problem at once.
func (p *Plugin) Validate() error {
	var errs []error
	if p.APIVersion != CurrentAPIVersion {
		errs = append(errs, fmt.Errorf("apiVersion: want %q, got %q", CurrentAPIVersion, p.APIVersion))
	}
	if p.Kind != Kind {
		errs = append(errs, fmt.Errorf("kind: want %q, got %q", Kind, p.Kind))
	}
	if !nameRE.MatchString(p.Metadata.Name) {
		errs = append(errs, fmt.Errorf("metadata.name: must match %s", nameRE.String()))
	}
	if !versionRE.MatchString(p.Metadata.Version) {
		errs = append(errs, fmt.Errorf("metadata.version: must be SemVer"))
	}
	if strings.TrimSpace(p.Metadata.Vendor) == "" {
		errs = append(errs, errors.New("metadata.vendor: required"))
	}
	if strings.TrimSpace(p.Spec.Description) == "" {
		errs = append(errs, errors.New("spec.description: required"))
	}
	if !validCategories[p.Spec.Category] {
		errs = append(errs, fmt.Errorf("spec.category: invalid %q", p.Spec.Category))
	}
	if p.Spec.DisplayCategory != "" && !validDisplayCategories[p.Spec.DisplayCategory] {
		errs = append(errs, fmt.Errorf("spec.displayCategory: invalid %q", p.Spec.DisplayCategory))
	}
	if len(p.Spec.Tags) > MaxTags {
		errs = append(errs, fmt.Errorf("spec.tags: max %d tags, got %d", MaxTags, len(p.Spec.Tags)))
	}
	for i, t := range p.Spec.Tags {
		if len(t) > MaxTagLength {
			errs = append(errs, fmt.Errorf("spec.tags[%d]: max %d chars, got %d", i, MaxTagLength, len(t)))
			continue
		}
		if !tagRE.MatchString(t) {
			errs = append(errs, fmt.Errorf("spec.tags[%d]: must match %s, got %q", i, tagRE.String(), t))
		}
	}
	switch p.Spec.Deployment.Type {
	case DeploymentHelm:
		if p.Spec.Deployment.Chart == "" {
			errs = append(errs, errors.New("spec.deployment.chart: required when type=helm"))
		}
	case DeploymentSystemd:
		if p.Spec.Deployment.Unit == "" {
			errs = append(errs, errors.New("spec.deployment.unit: required when type=systemd"))
		}
	default:
		errs = append(errs, fmt.Errorf("spec.deployment.type: must be helm or systemd, got %q", p.Spec.Deployment.Type))
	}

	allowed := privilegedNeedsByCategory[p.Spec.Category]
	for i, n := range p.Spec.Needs {
		if !allowed[n.Kind] {
			errs = append(errs, fmt.Errorf("spec.needs[%d]: category %q may not claim %q", i, p.Spec.Category, n.Kind))
			continue
		}
		switch n.Kind {
		case NeedDataset:
			if n.Dataset == nil || n.Dataset.Pool == "" || n.Dataset.Name == "" {
				errs = append(errs, fmt.Errorf("spec.needs[%d].dataset: pool+name required", i))
			}
		case NeedOIDCClient:
			if n.OIDCClient == nil || n.OIDCClient.ClientID == "" {
				errs = append(errs, fmt.Errorf("spec.needs[%d].oidcClient: clientId required", i))
			}
		case NeedTLSCert:
			if n.TLSCert == nil || n.TLSCert.CommonName == "" {
				errs = append(errs, fmt.Errorf("spec.needs[%d].tlsCert: commonName required", i))
			}
		case NeedPermission:
			if n.Permission == nil || n.Permission.Role == "" {
				errs = append(errs, fmt.Errorf("spec.needs[%d].permission: role required", i))
			}
		default:
			errs = append(errs, fmt.Errorf("spec.needs[%d].kind: unknown %q", i, n.Kind))
		}
	}

	for i, r := range p.Spec.API.Routes {
		if !strings.HasPrefix(r.Path, "/") {
			errs = append(errs, fmt.Errorf("spec.api.routes[%d].path: must start with /", i))
		}
		if r.Upstream == "" {
			errs = append(errs, fmt.Errorf("spec.api.routes[%d].upstream: required", i))
		}
		switch r.Auth {
		case AuthBearerPassthrough, AuthServiceToken:
		default:
			errs = append(errs, fmt.Errorf("spec.api.routes[%d].auth: must be bearer-passthrough or service-token", i))
		}
	}
	for i, d := range p.Spec.Dependencies {
		if !nameRE.MatchString(d.Name) {
			errs = append(errs, fmt.Errorf("spec.dependencies[%d].name: must match %s", i, nameRE.String()))
		}
		if !validDependencySources[d.Source] {
			errs = append(errs, fmt.Errorf("spec.dependencies[%d].source: must be tier-2 or bundled, got %q", i, d.Source))
		}
		if d.VersionConstraint != "" {
			if _, err := semver.NewConstraint(d.VersionConstraint); err != nil {
				errs = append(errs, fmt.Errorf("spec.dependencies[%d].versionConstraint: %v", i, err))
			}
		}
		if d.Name == p.Metadata.Name {
			errs = append(errs, fmt.Errorf("spec.dependencies[%d]: a plugin cannot depend on itself", i))
		}
	}

	if p.Spec.UI.Window.Bundle != "" {
		if p.Spec.UI.Window.Name == "" || p.Spec.UI.Window.Route == "" {
			errs = append(errs, errors.New("spec.ui.window: name+route required when bundle is set"))
		}
	}
	if len(errs) == 0 {
		return nil
	}
	return errors.Join(errs...)
}
