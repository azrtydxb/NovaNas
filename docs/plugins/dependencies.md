# Plugin dependencies

This guide covers the Tier 2 plugin dependency system: how to declare
dependencies in a manifest, how the engine resolves and installs them,
and how operators interact with the install/uninstall guarantees.

## Why dependencies?

A NovaNAS Tier 2 plugin can declare prerequisites on:

- **Other plugins** — for example, a "data warehouse UI" plugin
  declaring it needs the `object-storage` plugin installed first.
- **NovaNAS core features** — for example, a backup plugin declaring
  it needs the core ZFS replication subsystem available.

The engine uses these declarations to:

1. Recursively install missing plugin dependencies before the
   requested plugin's own provisioning runs.
2. Refuse to uninstall a plugin that other installed plugins still
   depend on (unless the operator forces the uninstall).
3. Render the dependency graph in Aurora's install consent dialog so
   operators can see what is about to be pulled in.

## Manifest authoring

Add a `dependencies` block under `spec`:

```yaml
apiVersion: novanas.io/v1
kind: Plugin
metadata:
  name: data-warehouse
  version: 2.0.0
  vendor: ACME
spec:
  description: Querying UI for data on object storage
  category: developer
  deployment:
    type: helm
    chart: chart/
  dependencies:
    - name: object-storage
      versionConstraint: ">=1.0.0,<2.0.0"
      source: tier-2
    - name: zfs-replication
      source: bundled
```

### Fields

| Field | Required | Description |
|-------|----------|-------------|
| `name` | yes | Plugin name. Same DNS-1123 rules as `metadata.name`. |
| `versionConstraint` | no | SemVer constraint (see below). Empty matches any version. |
| `source` | yes | `tier-2` (marketplace plugin) or `bundled` (NovaNAS core feature). |

### Version constraints

Constraints use the [Masterminds SemVer
syntax](https://github.com/Masterminds/semver#checking-version-constraints):

| Operator | Meaning |
|----------|---------|
| `=1.2.3` or `1.2.3` | exactly 1.2.3 |
| `>=1.0.0` | 1.0.0 and up |
| `<2.0.0` | strictly less than 2.0.0 |
| `~1.2` | tilde range: 1.2.0 to 1.99.99 |
| `^1.2` | caret range: 1.2.0 to 1.x but not 2.x |
| `>=1.0.0,<2.0.0` | AND-of-clauses (comma-separated) |

The manifest validator rejects manifests with malformed constraints,
self-references, or unknown source values.

### Choosing a source

**`tier-2`** — the dep is published on a marketplace the engine
consults. Installing the plugin will fetch + verify + install the dep
recursively. Uninstalling the dep is then blocked while the parent is
still installed.

**`bundled`** — the dep is a NovaNAS core feature (e.g. ZFS
replication, NFSv4 ACL support). Bundled deps are documentation-only:
the engine never installs them, but Aurora surfaces them in the
install dialog ("requires NovaNAS core feature: ZFS replication") so
the operator can confirm the host meets the prerequisite. v1 accepts
any name as a bundled dep; a future release will validate against a
known list of core capabilities.

## Recursive install behaviour

When the operator runs `POST /api/v1/plugins {name, version}`:

1. The engine fetches and verifies the requested plugin's tarball.
2. It parses the manifest and runs the resolver against
   `spec.dependencies`.
3. For each `tier-2` dep:
   - If a satisfying version is already installed: noted in the
     install plan as `skip`.
   - If installed at an unsatisfiable version: the call **fails**
     with HTTP 409 and a clear message:
     `plugin "data-warehouse" requires "object-storage" >=1.0.0,<2.0.0; object-storage@2.1.0 is currently installed; upgrade or remove object-storage first.`
     The engine never auto-upgrades or auto-downgrades.
   - Otherwise: the highest version satisfying every active
     constraint is selected from the marketplace and installed via a
     recursive `Install` call.
4. For each `bundled` dep: noted in the install plan as `bundled`,
   never installed.
5. After every dep is in place the requested plugin's own
   provisioning runs.

The resolver detects cycles (a plugin depending on itself, or a chain
that eventually loops back) and refuses to install cyclic graphs.

### Partial-state behaviour

If a dependency install fails midway through a recursive install, the
deps that already succeeded are **not** rolled back. They may be
useful on their own, and the operator may simply retry the failed
install once the underlying issue is fixed. The partial state is
audit-logged.

### Plan response

The Install response includes an `installedDeps` array that lists the
dep steps the engine walked. Aurora shows this in the post-install
toast so the operator sees exactly what was pulled in.

## Uninstall guard

The engine refuses to uninstall a plugin whose manifests still appear
in another installed plugin's `spec.dependencies`. The DELETE call
returns:

```http
HTTP/1.1 409 Conflict
Content-Type: application/json

{
  "error": "has_dependents",
  "message": "plugins: \"object-storage\" has dependents: data-warehouse, photos-app",
  "plugin": "object-storage",
  "blockedBy": ["data-warehouse", "photos-app"]
}
```

Operators that knowingly want to break the dependency relation pass
`?force=true`:

```bash
curl -X DELETE "/api/v1/plugins/object-storage?force=true" -H "Authorization: Bearer …"
```

The forced uninstall succeeds and the dependents that were broken are
audit-logged with `breaking dependents=[…]`. Aurora's confirmation
dialog should call out the impact before forcing.

## Read-only inspection endpoints

Both endpoints are open to any identity with `nova:plugins:read`.

### `GET /api/v1/plugins/{name}/dependencies`

Returns a depth-first tree and a flat install plan for the plugin. If
the plugin is installed the stored manifest is used; otherwise the
marketplace is consulted (pass `?version=…` to pin a marketplace
version when the plugin isn't installed).

The response shape:

```json
{
  "tree": {
    "name": "data-warehouse",
    "version": "2.0.0",
    "source": "tier-2",
    "children": [
      {
        "name": "object-storage",
        "constraint": ">=1.0.0,<2.0.0",
        "source": "tier-2",
        "installed": true,
        "satisfied": true,
        "version": "1.5.2"
      },
      {
        "name": "zfs-replication",
        "source": "bundled",
        "satisfied": true
      }
    ]
  },
  "plan": [
    { "name": "object-storage", "version": "1.5.2", "action": "skip", "constraint": ">=1.0.0,<2.0.0", "source": "tier-2" },
    { "name": "data-warehouse", "version": "2.0.0", "action": "install", "source": "tier-2" }
  ]
}
```

Aurora renders this as the install consent dialog ("the following
will be pulled in / are already present").

### `GET /api/v1/plugins/{name}/dependents`

Returns the names of installed plugins that list `name` as a tier-2
dependency. Useful for the uninstall confirmation modal:

```json
{
  "plugin": "object-storage",
  "dependents": ["data-warehouse", "photos-app"]
}
```

## Operator runbook

**"My install failed because a dep is at the wrong version."**
Either upgrade the existing plugin to a version that satisfies the
constraint:

```bash
curl -X PATCH "/api/v1/plugins/object-storage" -d '{"version":"1.5.2"}'
```

…or remove the older plugin (after considering its own dependents)
and let the engine pull in the right version on the next install.

**"I want to clean up everything."**
Walk dependents bottom-up: query
`/api/v1/plugins/{name}/dependents` for each plugin you want to
remove and uninstall its dependents first.

**"I really want to nuke this plugin even though things depend on
it."** Pass `?force=true` to DELETE. The dependents will be left in
a broken state — they'll start failing health checks the moment they
try to talk to the plugin you removed. Audit log captures the
operator and the broken set.

**"I'm authoring a plugin and I want to know what core features are
allowed as bundled deps."**
v1 accepts any name. The Aurora install dialog will display whatever
string you put there as "requires NovaNAS core feature: $name". A
future release will tighten this against a known list — see
`internal/plugins/manifest.go` for the planned hook
(`validDependencySources`).

## Library choice

The resolver uses
[`github.com/Masterminds/semver/v3`](https://pkg.go.dev/github.com/Masterminds/semver/v3)
for constraint matching. Helm pulls it in as a transitive dep so it
adds zero new dependencies to the module graph.
`golang.org/x/mod/semver` was considered but only supports raw
comparison, not the range constraint syntax operators expect (`~`,
`^`, AND-of-clauses).
