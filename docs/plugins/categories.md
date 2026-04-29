# Plugin Categories — Privilege vs Display

NovaNAS Tier 2 plugins carry two orthogonal category fields. Both
appear under `spec:` in the manifest, but they answer different
questions and are evaluated by different parts of the system.

| Field             | Axis      | Who reads it             | What it controls                                           |
|-------------------|-----------|--------------------------|------------------------------------------------------------|
| `category`        | Privilege | The plugin engine        | Which `needs:` kinds the plugin may claim                  |
| `displayCategory` | UX        | Aurora's App Center      | Which sidebar group the plugin appears under               |
| `tags`            | UX        | Aurora's App Center      | Free-form filtering ("s3", "backup-target", "4k", …)       |

Authors should pick the right value for each axis independently.
A backup tool that needs `dataset` access is `category: storage`
(privilege) but `displayCategory: backup` (UX).

## Privilege axis: `category`

`category` is the existing field the engine has always shipped. Its
sole job is to gate which `needs:` kinds a plugin is allowed to
auto-provision at install time. The current matrix:

| `category`      | Allowed `needs:` kinds                                |
|-----------------|--------------------------------------------------------|
| `storage`       | `dataset`, `oidcClient`, `tlsCert`, `permission`       |
| `networking`    | `oidcClient`, `tlsCert`, `permission`                  |
| `observability` | `oidcClient`, `tlsCert`, `permission`                  |
| `developer`     | `oidcClient`, `permission`                             |
| `utility`       | `permission`                                            |

A manifest that asks for a `dataset` while declaring `category: utility`
is rejected at parse time. This is the privilege-escalation guard
called out in the engine spec — it is non-negotiable and the
operator/user cannot override it.

## Display axis: `displayCategory`

`displayCategory` is purely a UX hint. It tells Aurora which group in
the App Center sidebar the plugin belongs to. The engine ignores it
when deciding whether to permit `needs:` claims.

The 14 valid values are:

| Value            | Intent                                                                |
|------------------|-----------------------------------------------------------------------|
| `backup`         | Backup tools (Restic, Duplicati, snapshot orchestrators)              |
| `files`          | File browsers, sync tools, file-sharing surfaces                      |
| `multimedia`     | Video/audio servers (Jellyfin, Plex, Navidrome)                       |
| `photos`         | Photo management (PhotoPrism, Immich)                                 |
| `productivity`   | Office suites, kanban boards, note-taking                             |
| `security`       | Auth proxies, VPN, password managers                                  |
| `communication`  | Chat, email, conferencing                                             |
| `home`           | Smart-home / IoT (Home Assistant, Mosquitto)                          |
| `developer`      | CI/CD, code-hosting, registries                                       |
| `network`        | DNS, DHCP, reverse proxies, monitoring of network paths               |
| `storage`        | Object stores, NFS/CIFS overlays, dataset tooling                     |
| `surveillance`   | NVR / camera management (Frigate, Shinobi)                            |
| `utilities`      | Catch-all for system tooling without a clearer home                   |
| `observability`  | Metrics, logs, tracing dashboards                                     |

Plugins without an explicit `displayCategory` get one inferred from
their privilege `category` via this default mapping:

| Privilege `category` | Inferred `displayCategory` |
|----------------------|----------------------------|
| `storage`            | `storage`                  |
| `networking`         | `network`                  |
| `observability`      | `observability`            |
| `developer`          | `developer`                |
| `utility`            | `utilities`                |
| _(anything else)_    | _(empty — Aurora groups under "Other")_ |

The fill-in happens after YAML decode and before validation, so a
manifest that omits the field always validates as if the inferred
value had been written explicitly.

## Tags

`tags` is a free-form array of short, machine-friendly labels for
fine-grained filtering. Aurora exposes them as filter chips in the App
Center.

Rules:

- Each tag matches `^[a-z0-9][a-z0-9-]*$` (lowercase alphanumeric +
  dashes; must start with alphanumeric)
- Max 32 characters per tag
- Max 16 tags per plugin

The cap on tag count is intentional — tags ride along in the merged
marketplace index, so an unbounded array would inflate every catalog
fetch and slow down the App Center for every operator.

## Example — object-storage

```yaml
apiVersion: novanas.io/v1
kind: Plugin
metadata:
  name: object-storage
  version: 1.0.0
  vendor: novanas.io
spec:
  description: "S3-compatible object storage powered by RustFS."
  category: storage          # privilege axis — needs dataset+oidcClient+tlsCert
  displayCategory: storage   # UX axis — appears under "Storage" in Aurora
  tags: ["s3", "object", "backup-target", "rustfs"]
  ...
```

## Example — backup tool that lives in the `storage` privilege class

```yaml
spec:
  description: "Restic-based backup orchestrator."
  category: storage          # needs dataset access for snapshots
  displayCategory: backup    # but Aurora groups it under "Backup"
  tags: ["restic", "snapshot", "encryption"]
```

## API surface

- `GET /plugins/index?displayCategory=storage` — single-valued filter,
  rejects unknown categories with 400
- `GET /plugins/index?tag=s3&tag=object` — repeated, AND semantics
- `GET /plugins/categories` — returns all 14 categories with plugin
  counts (zero-count entries included so the sidebar layout stays
  stable)
