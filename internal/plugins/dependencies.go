// Package plugins — dependency resolver for the Tier 2 plugin engine.
//
// The Resolver walks a plugin's declared `spec.dependencies` graph,
// figures out which deps need installing (and at what version), and
// returns an ordered install plan with the deepest deps first and the
// root plugin last. It does NOT perform installs — that is the
// lifecycle code's job. Keeping the resolver pure makes it cheap to
// preview an install plan from an HTTP handler.
//
// Constraint matching uses github.com/Masterminds/semver/v3 because it
// is already in the module graph (helm pulls it transitively) and it
// supports the rich constraint syntax callers expect: ">=1.0.0",
// "<2.0.0", "~1.2", "=1.2.3", and AND-of-clauses via comma:
// ">=1.0.0,<2.0.0". golang.org/x/mod/semver was considered but only
// supports raw comparison, not range constraints.
package plugins

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Masterminds/semver/v3"
)

// ErrCycle is returned when the dependency graph contains a cycle. It
// is wrapped with details about the offending path.
var ErrCycle = errors.New("plugins: dependency cycle")

// ErrUnsatisfiable is returned when an installed plugin's version sits
// outside the constraint required by a parent. The engine refuses to
// auto-upgrade or auto-downgrade — the operator resolves manually.
var ErrUnsatisfiable = errors.New("plugins: dependency version unsatisfiable")

// DependencyLookup is the marketplace surface the resolver needs. The
// production implementation is *MarketplaceClient; tests pass a fake.
type DependencyLookup interface {
	FindVersion(ctx context.Context, name, version string) (*IndexPlugin, *IndexVersion, error)
}

// InstalledLookup reports the version of a plugin that is currently
// installed (or "" + false if it isn't). The production implementation
// is a thin wrapper around the plugins DB; tests pass a map.
type InstalledLookup interface {
	InstalledVersion(ctx context.Context, name string) (string, bool, error)
}

// PlanStep is one node of the resolver's output.
type PlanStep struct {
	// Name is the plugin name.
	Name string `json:"name"`
	// Version is the version that will be installed. For Action=skip it
	// is the currently-installed version (which already satisfies).
	Version string `json:"version"`
	// Source is "tier-2" or "bundled". Bundled steps never produce work.
	Source DependencySource `json:"source"`
	// Action is "install" (fetch + install) or "skip" (already at a
	// satisfying version) or "bundled" (no-op, documentation only).
	Action string `json:"action"`
	// Constraint is the version constraint that selected this step,
	// useful for surfacing "why" in Aurora's install dialog.
	Constraint string `json:"constraint,omitempty"`
}

// PlanActionInstall, etc. — the legal Action values.
const (
	PlanActionInstall = "install"
	PlanActionSkip    = "skip"
	PlanActionBundled = "bundled"
)

// DependencyTreeNode is a tree representation of the dep graph for the
// /plugins/{name}/dependencies endpoint. Children are this node's
// direct dependencies; the tree is walked depth-first.
type DependencyTreeNode struct {
	Name       string               `json:"name"`
	Version    string               `json:"version,omitempty"`
	Constraint string               `json:"constraint,omitempty"`
	Source     DependencySource     `json:"source"`
	Installed  bool                 `json:"installed"`
	Satisfied  bool                 `json:"satisfied"`
	Children   []DependencyTreeNode `json:"children,omitempty"`
}

// Resolver computes install plans + dependency trees from a parsed
// plugin manifest. It is stateless; construct one per request.
type Resolver struct {
	Marketplace DependencyLookup
	Installed   InstalledLookup
	// MaxDepth caps recursion so a malicious manifest can't tank the
	// process. 32 is generous — real graphs will have <5 levels.
	MaxDepth int
}

// NewResolver constructs a Resolver. MaxDepth defaults to 32.
func NewResolver(market DependencyLookup, installed InstalledLookup) *Resolver {
	return &Resolver{Marketplace: market, Installed: installed, MaxDepth: 32}
}

// Plan returns an ordered list of steps to install root.
// The last entry is always root itself. Earlier entries are deps in
// install-safe order (deepest first). The resolver merges duplicate
// names: if two parents both depend on plugin X, X appears once with
// the highest version satisfying ALL active constraints.
//
// Plan does NOT modify any state — it only reads from Marketplace and
// Installed. Callers (lifecycle.Install) then walk the plan, calling
// Install for each Action=install step.
func (r *Resolver) Plan(ctx context.Context, root *Plugin) ([]PlanStep, error) {
	if root == nil {
		return nil, errors.New("plugins: resolver: root manifest is nil")
	}
	st := &resolveState{
		r:           r,
		visiting:    map[string]bool{},
		resolved:    map[string]*PlanStep{},
		order:       []string{},
		constraints: map[string][]string{},
	}
	if err := st.walk(ctx, root, 0); err != nil {
		return nil, err
	}
	out := make([]PlanStep, 0, len(st.order)+1)
	for _, name := range st.order {
		step := st.resolved[name]
		if step == nil {
			continue
		}
		out = append(out, *step)
	}
	// Root is always the last step.
	out = append(out, PlanStep{
		Name:    root.Metadata.Name,
		Version: root.Metadata.Version,
		Source:  DependencySourceTier2,
		Action:  PlanActionInstall,
	})
	return out, nil
}

// Tree returns a depth-first snapshot of the dep graph rooted at the
// supplied manifest. Cycles abort with ErrCycle.
func (r *Resolver) Tree(ctx context.Context, root *Plugin) (*DependencyTreeNode, error) {
	if root == nil {
		return nil, errors.New("plugins: resolver: root manifest is nil")
	}
	visiting := map[string]bool{}
	node, err := r.tree(ctx, root, visiting, 0)
	if err != nil {
		return nil, err
	}
	return node, nil
}

func (r *Resolver) tree(ctx context.Context, m *Plugin, visiting map[string]bool, depth int) (*DependencyTreeNode, error) {
	if depth > r.maxDepth() {
		return nil, fmt.Errorf("plugins: resolver: max depth %d exceeded at %q", r.maxDepth(), m.Metadata.Name)
	}
	if visiting[m.Metadata.Name] {
		return nil, fmt.Errorf("%w: %s", ErrCycle, m.Metadata.Name)
	}
	visiting[m.Metadata.Name] = true
	defer delete(visiting, m.Metadata.Name)

	node := &DependencyTreeNode{
		Name:    m.Metadata.Name,
		Version: m.Metadata.Version,
		Source:  DependencySourceTier2,
	}
	for _, dep := range m.Spec.Dependencies {
		child := DependencyTreeNode{
			Name:       dep.Name,
			Constraint: dep.VersionConstraint,
			Source:     dep.Source,
		}
		if dep.Source == DependencySourceBundled {
			// Bundled deps are documentation-only; we don't recurse.
			child.Satisfied = true
			node.Children = append(node.Children, child)
			continue
		}
		// Installed?
		if r.Installed != nil {
			if cur, ok, err := r.Installed.InstalledVersion(ctx, dep.Name); err == nil && ok {
				child.Installed = true
				child.Version = cur
				child.Satisfied = constraintSatisfied(dep.VersionConstraint, cur)
			}
		}
		// Recurse via marketplace lookup (best-effort: missing entries leave
		// the subtree empty rather than failing the whole tree).
		if r.Marketplace != nil {
			_, ver, err := r.Marketplace.FindVersion(ctx, dep.Name, "")
			if err == nil && ver != nil {
				if child.Version == "" {
					child.Version = ver.Version
				}
				// We can't easily fetch the dep's own manifest here without
				// downloading the tarball, which the resolver intentionally
				// avoids. Tree depth therefore stops at the marketplace
				// entry. The /dependencies endpoint can be augmented later
				// with a deeper crawl if Aurora needs it.
			}
		}
		node.Children = append(node.Children, child)
	}
	return node, nil
}

// resolveState carries DFS bookkeeping across the recursion.
type resolveState struct {
	r           *Resolver
	visiting    map[string]bool        // names on the current DFS stack — cycle detection
	resolved    map[string]*PlanStep   // name → final plan step (deduped)
	order       []string               // installation order (excludes root)
	constraints map[string][]string    // name → list of constraints accumulated from parents
}

func (st *resolveState) walk(ctx context.Context, m *Plugin, depth int) error {
	if depth > st.r.maxDepth() {
		return fmt.Errorf("plugins: resolver: max depth %d exceeded at %q", st.r.maxDepth(), m.Metadata.Name)
	}
	if st.visiting[m.Metadata.Name] {
		return fmt.Errorf("%w: %s", ErrCycle, m.Metadata.Name)
	}
	st.visiting[m.Metadata.Name] = true
	defer delete(st.visiting, m.Metadata.Name)

	for _, dep := range m.Spec.Dependencies {
		if dep.Source == DependencySourceBundled {
			// Bundled deps are satisfied implicitly; record once so Aurora
			// can still display them in the install plan.
			if _, ok := st.resolved[dep.Name]; !ok {
				st.resolved[dep.Name] = &PlanStep{
					Name:       dep.Name,
					Source:     DependencySourceBundled,
					Action:     PlanActionBundled,
					Constraint: dep.VersionConstraint,
				}
				st.order = append(st.order, dep.Name)
			}
			continue
		}
		if dep.Source != DependencySourceTier2 {
			return fmt.Errorf("plugins: resolver: %q: unknown dependency source %q", dep.Name, dep.Source)
		}
		if dep.Name == m.Metadata.Name {
			return fmt.Errorf("%w: %s self-references", ErrCycle, dep.Name)
		}
		st.constraints[dep.Name] = append(st.constraints[dep.Name], dep.VersionConstraint)

		// 1. Already installed?
		if st.r.Installed != nil {
			if cur, ok, err := st.r.Installed.InstalledVersion(ctx, dep.Name); err != nil {
				return fmt.Errorf("plugins: resolver: installed lookup for %q: %w", dep.Name, err)
			} else if ok {
				if !constraintSatisfied(dep.VersionConstraint, cur) {
					return fmt.Errorf("%w: plugin %q requires %q %s; %s@%s is currently installed; upgrade or remove %s first",
						ErrUnsatisfiable, m.Metadata.Name, dep.Name, formatConstraint(dep.VersionConstraint), dep.Name, cur, dep.Name)
				}
				// Satisfying version present — record skip and do not recurse.
				if existing, ok := st.resolved[dep.Name]; !ok {
					st.resolved[dep.Name] = &PlanStep{
						Name:       dep.Name,
						Version:    cur,
						Source:     DependencySourceTier2,
						Action:     PlanActionSkip,
						Constraint: dep.VersionConstraint,
					}
					st.order = append(st.order, dep.Name)
				} else if existing.Action == PlanActionSkip && !constraintSatisfied(dep.VersionConstraint, existing.Version) {
					return fmt.Errorf("%w: %q version %s does not satisfy combined constraints", ErrUnsatisfiable, dep.Name, existing.Version)
				}
				continue
			}
		}

		// 2. Resolve from marketplace.
		if st.r.Marketplace == nil {
			return fmt.Errorf("plugins: resolver: marketplace not configured; cannot resolve %q", dep.Name)
		}
		idxPlugin, _, err := st.r.Marketplace.FindVersion(ctx, dep.Name, "")
		if err != nil {
			return fmt.Errorf("plugins: resolver: marketplace lookup %q: %w", dep.Name, err)
		}
		picked, err := pickHighestSatisfying(idxPlugin.Versions, st.constraints[dep.Name])
		if err != nil {
			return fmt.Errorf("plugins: resolver: %q: %w", dep.Name, err)
		}

		// 3. Record the install step (idempotent).
		if existing, ok := st.resolved[dep.Name]; ok {
			// Combined constraint may have narrowed further. Re-pick.
			if existing.Action == PlanActionInstall && !constraintSatisfied(dep.VersionConstraint, existing.Version) {
				existing.Version = picked
				existing.Constraint = strings.Join(filterEmpty(st.constraints[dep.Name]), ",")
			}
		} else {
			st.resolved[dep.Name] = &PlanStep{
				Name:       dep.Name,
				Version:    picked,
				Source:     DependencySourceTier2,
				Action:     PlanActionInstall,
				Constraint: strings.Join(filterEmpty(st.constraints[dep.Name]), ","),
			}
			st.order = append(st.order, dep.Name)
		}

		// 4. Recurse — but only if the dep itself declares deps. The
		//    resolver intentionally avoids downloading tarballs at plan
		//    time. If the marketplace index begins to surface declared
		//    deps inline in the future, this is the hook point.
		// For v1, transitive deps inside marketplace plugins are
		// discovered on each install (the recursive Install call in
		// lifecycle re-runs the resolver against the freshly-fetched
		// child manifest). The plan therefore lists direct deps only;
		// transitive deps are surfaced via /plugins/{name}/dependencies
		// once those plugins are also installed.
	}
	return nil
}

func (r *Resolver) maxDepth() int {
	if r == nil || r.MaxDepth <= 0 {
		return 32
	}
	return r.MaxDepth
}

// constraintSatisfied returns true when ver satisfies constraint.
// An empty constraint matches anything (the manifest validator already
// rejected malformed strings, so any non-empty value is well-formed).
func constraintSatisfied(constraint, ver string) bool {
	if strings.TrimSpace(constraint) == "" {
		return true
	}
	c, err := semver.NewConstraint(constraint)
	if err != nil {
		return false
	}
	v, err := semver.NewVersion(ver)
	if err != nil {
		return false
	}
	return c.Check(v)
}

// pickHighestSatisfying returns the highest version among versions that
// satisfies every supplied constraint. Versions are listed by the
// marketplace newest-first (per existing convention) but the function
// does its own ordering to be safe.
func pickHighestSatisfying(versions []IndexVersion, constraints []string) (string, error) {
	cs := filterEmpty(constraints)
	var combined *semver.Constraints
	if len(cs) > 0 {
		c, err := semver.NewConstraint(strings.Join(cs, ","))
		if err != nil {
			return "", fmt.Errorf("invalid constraint %q: %w", strings.Join(cs, ","), err)
		}
		combined = c
	}
	var best *semver.Version
	var bestRaw string
	for _, v := range versions {
		parsed, err := semver.NewVersion(v.Version)
		if err != nil {
			continue
		}
		if combined != nil && !combined.Check(parsed) {
			continue
		}
		if best == nil || parsed.GreaterThan(best) {
			best = parsed
			bestRaw = v.Version
		}
	}
	if best == nil {
		return "", fmt.Errorf("no version satisfies %s", strings.Join(cs, ","))
	}
	return bestRaw, nil
}

// formatConstraint returns a human-friendly rendering for error
// messages — empty constraint becomes "(any)".
func formatConstraint(c string) string {
	if strings.TrimSpace(c) == "" {
		return "(any version)"
	}
	return c
}

func filterEmpty(in []string) []string {
	out := make([]string, 0, len(in))
	for _, s := range in {
		if strings.TrimSpace(s) != "" {
			out = append(out, s)
		}
	}
	return out
}
