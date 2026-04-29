# Plugin pre-install manifest preview

`GET /api/v1/plugins/index/{name}/manifest?version={semver}` returns the
parsed manifest and a structured permissions summary for a marketplace
plugin **without** installing it. It is the data source behind Aurora's
"Install" consent dialog (the same role Synology / QNAP play on their
package centers).

The endpoint is read-only — it does not touch the database, the on-disk
plugin tree, or any runtime substrate (systemd / helm). It only:

1. Looks up `(name, version)` in the marketplace index.
2. Downloads the tarball + signature.
3. Cosign-verifies the signature.
4. Reads `manifest.yaml` from the tarball root in memory.
5. Validates the manifest and derives a `PermissionsSummary`.

Permission required: `nova:plugins:read` (the same gate as the rest of
the marketplace browsing endpoints).

## Typical Aurora flow

```
1. User opens Marketplace        →  GET /plugins/index
2. User picks "rustfs 1.2.3"     →  GET /plugins/index/rustfs
3. User clicks Install           →  GET /plugins/index/rustfs/manifest?version=1.2.3
   ↓ Aurora renders consent dialog from the response
4. User confirms                 →  POST /plugins {"name":"rustfs","version":"1.2.3"}
   ↓ engine fetches+verifies AGAIN, runs needs+deploy, writes audit row
     including the previewed permissions summary so the consent record
     is exact
```

The preview response is intentionally not cached aggressively. Tarballs
are tiny and the engine always re-fetches on Install — comparing the
`tarballSha256` field of the preview against the install-time download
guarantees nothing changed in between.

## Response shape

```json
{
  "manifest": { /* the full Plugin object — apiVersion, kind, metadata, spec */ },
  "permissions": {
    "willCreate": [
      {"kind": "dataset",     "what": "ZFS dataset tank/objects",                          "destructive": false},
      {"kind": "tlsCert",     "what": "TLS cert for rustfs.novanas.local",                 "destructive": false},
      {"kind": "oidcClient",  "what": "Keycloak client \"rustfs\"",                        "destructive": false},
      {"kind": "permission",  "what": "Bind realm role \"nova-operator\" to plugin service account", "destructive": false}
    ],
    "willMount":  ["/api/v1/plugins/rustfs/admin/*"],
    "willOpen":   [],
    "scopes":     ["PermPluginsRead"],
    "category":   "storage"
  },
  "tarballSha256": "0123…"
}
```

### `permissions.willCreate`

One entry per `spec.needs[*]` in the manifest, in declaration order.
Aurora groups these into a single "this app will create:" section in
the consent dialog.

- `kind` — one of `dataset`, `oidcClient`, `tlsCert`, `permission`. Aurora
  keys icons + sub-grouping off this.
- `what` — one-line human-readable description, suitable for a bullet
  list. Generated server-side from the manifest fields (`pool/name`,
  `commonName`, `clientId`, `role`).
- `destructive` — `true` when the resource mutates state the plugin
  does NOT own (e.g. reuses an existing dataset, binds a global role).
  **For v1 this is always `false`** — the engine creates fresh
  resources scoped to the plugin. The field exists so future manifest
  features (claim-existing-dataset, bind-existing-role) can opt in
  without an API break.

### `permissions.willMount`

Lists every nova-api path the engine will register on Install. One
entry per `spec.api.routes[*].path`, formatted as
`/api/v1/plugins/{name}{route.path}/*`. The trailing `/*` makes it
explicit that the engine catches every subpath under the route.

Aurora's consent dialog renders this as "this app will add the
following endpoints to nova-api:".

### `permissions.willOpen`

Reserved for future port-allocation declarations. v1 manifests do not
declare ports, so this field is always `[]`. Future manifest schemas
may add a `spec.deployment.ports:` block; when they do the field will
populate without an API break — current Aurora consent dialogs already
handle the empty case gracefully.

### `permissions.scopes`

The nova-api permissions a caller needs to install + later interact
with this plugin. v1 surfaces only `PermPluginsRead`; admin actions
(install/uninstall/upgrade) are gated by `PermPluginsAdmin` server-side
and not listed here (since the user clicking Install already had to
have it to see the dialog).

### `permissions.category`

Mirrors `spec.category` — Aurora groups apps by it (Storage, Networking,
Observability, Developer, Utility).

### `tarballSha256`

Hex SHA256 of the cosign-verified tarball bytes. Captured so the audit
row written at Install confirm-time pins EXACTLY which artifact the
user previewed. Operators auditing a confirmed install can re-derive
the SHA from the tarball URL and compare.

## Error responses

| Status | Code                       | Meaning                                      |
|--------|----------------------------|----------------------------------------------|
| 400    | `missing_version`          | `?version=` query parameter is required.     |
| 404    | `not_found`                | Plugin or version not in the marketplace index. |
| 422    | `signature_invalid`        | Cosign verification failed — package may be tampered. **Do not install.** |
| 422    | `manifest_invalid`         | manifest.yaml is missing, malformed, or its `metadata.name` does not match the URL `{name}`. |
| 502    | `marketplace_unreachable`  | The marketplace index host or artifact URL is unreachable. |
| 503    | `not_available`            | nova-api is running without a marketplace client wired (dev / partial deploy). |

Aurora should surface a tampering warning on `422 signature_invalid`
and block the Install button — that response is the only signal the
package author key has been compromised or the artifact has been
swapped on the CDN.

## Audit

Every successful preview emits a structured `plugins.preview` audit
row including the caller identity, plugin name, version, and the
verified `tarballSha256`. The global Audit middleware also records
the HTTP request itself; the structured row exists so audit consumers
can join `plugins.preview` to the subsequent `plugins.install` row by
SHA and confirm the operator consented to the same artifact that was
ultimately installed.

## SDK

Go SDK callers use `Client.PreviewPlugin`:

```go
preview, err := nc.PreviewPlugin(ctx, "rustfs", "1.2.3")
if err != nil { return err }
fmt.Println(preview.TarballSHA256)
for _, r := range preview.Permissions.WillCreate {
    fmt.Println(r.Kind, r.What)
}
```

`name` and `version` are both required; the SDK validates both client-
side before issuing the request.
