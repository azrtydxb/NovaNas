# Authoring a NovaNAS Tier 2 Plugin

A Tier 2 plugin is a NovaNAS-signed package that integrates with the
Aurora chrome at the API and UI level. This guide walks through the
manifest format, the package layout, the build/sign/publish flow,
and how to test against a local nova-api.

If you only need to package a Helm chart on the embedded k3s and do
NOT need API or UI integration with the NovaNAS shell, you want a
Tier 3 community app instead — see `internal/workloads/`.

## Package layout

A plugin is a `.tar.gz` containing:

```
manifest.yaml         # required, at the root
ui/                   # optional; React module + assets
  main.js
  style.css
chart/                # required if deployment.type = helm
  Chart.yaml
  values.yaml
  templates/
systemd/              # required if deployment.type = systemd
  <plugin>.service
hooks/                # optional; lifecycle scripts referenced from spec.lifecycle
  preInstall.sh
  postInstall.sh
  preUninstall.sh
```

A detached signature `<plugin>-<version>.tar.gz.sig` is published
alongside the tarball in the marketplace.

## The manifest

`manifest.yaml` — the contract. Validated by the engine on install;
malformed manifests are rejected with a list of every problem.

```yaml
apiVersion: novanas.io/v1
kind: Plugin
metadata:
  name: rustfs              # DNS-1123, [a-z0-9-], 1..40 chars
  version: 1.2.3            # SemVer; v-prefix optional
  vendor: NovaNAS Project
  signature: ""             # informational; the marketplace re-signs
spec:
  description: S3-compatible object storage
  category: storage         # storage | networking | observability | developer | utility
  icon: rustfs.svg
  deployment:
    type: helm              # helm | systemd
    chart: chart/           # helm: relative path inside the tarball
    namespace: rustfs       # helm: target k3s namespace
    # OR
    # type: systemd
    # unit: rustfs.service
  needs:
    - kind: dataset
      dataset:
        pool: tank
        name: rustfs/data
        properties:
          compression: lz4
    - kind: oidcClient
      oidcClient:
        clientId: rustfs
        redirectUris: ["https://nas.local/auth/cb"]
    - kind: tlsCert
      tlsCert:
        commonName: rustfs.local
        dnsNames: [rustfs.local, nas.local]
        ttlDays: 365
    - kind: permission
      permission:
        role: rustfs-admin
        description: Full RustFS bucket admin
  api:
    routes:
      - path: /buckets
        upstream: http://127.0.0.1:9000
        scopes: [s3:read, s3:write]
        auth: bearer-passthrough  # bearer-passthrough | service-token
  ui:
    window:
      name: RustFS
      icon: rustfs.svg
      route: /apps/rustfs
      bundle: main.js
  health:
    path: /healthz
    intervalSeconds: 30
    timeoutSeconds: 5
  lifecycle:
    preInstall: hooks/preInstall.sh
    postInstall: hooks/postInstall.sh
    preUninstall: hooks/preUninstall.sh
```

## Category gates

`spec.category` controls which `needs:` your plugin may claim. The
engine rejects manifests that try to claim privileged resources
outside their declared category — this is a defence-in-depth check
applied BEFORE signature verification ever happens.

| Category | dataset | oidcClient | tlsCert | permission |
|----------|---------|------------|---------|------------|
| `storage` | yes | yes | yes | yes |
| `networking` | no | yes | yes | yes |
| `observability` | no | yes | yes | yes |
| `developer` | no | yes | no | yes |
| `utility` | no | no | no | yes |

If you need a privilege your category doesn't grant, the right move
is to discuss the categorization with the NovaNAS team — not to
escalate the category in the manifest. Marketplace review will
reject mis-categorized plugins.

## API routes

Every route in `spec.api.routes` is mounted under
`/api/v1/plugins/<name>/api/<path>`. The router:

1. Authenticates the caller against Keycloak (the same chain every
   nova-api endpoint uses).
2. Checks the caller has `nova:plugins:read`.
3. Applies the manifest's `auth` mode:
   - `bearer-passthrough`: the caller's JWT is forwarded verbatim
     to the upstream. The upstream MUST verify it.
   - `service-token`: the caller's auth is stripped and a fresh
     service token is minted using the plugin's own oidcClient
     credentials. Use this when the upstream can't speak Keycloak.
4. Reverse-proxies to the upstream URL.

Pick `bearer-passthrough` whenever your upstream can verify a
Keycloak JWT — it preserves the caller's identity end-to-end and
lets your upstream apply its own RBAC.

## UI bundle

The `ui/` directory is unpacked to
`/var/lib/nova-nas/plugins/<name>/ui/` and served statically at
`/api/v1/plugins/<name>/ui/*`.

Aurora dynamically imports `/api/v1/plugins/<name>/ui/main.js` as an
ESM module and renders the named window when the operator clicks
your plugin's tile. The bundle should:

- Be a single-file ES module (no relative imports outside the bundle).
- Default-export a React component or a registration function.
- Load any extra assets (CSS, images) via relative URLs — the
  ui-assets server resolves them under your plugin's namespace.

The Aurora team publishes a tiny SDK with the chrome-side hooks
(window manager, theme, auth). Build against it with:

```sh
npm install @novanas/plugin-sdk
```

See the marketplace repo's RustFS plugin for a worked example.

## Lifecycle hooks

Hooks are executed by nova-api on the host with the plugin's
unprivileged uid. Each hook receives the plugin's root directory as
its CWD and the following env vars:

| Var | Value |
|-----|-------|
| `NOVANAS_PLUGIN_NAME` | The plugin's name. |
| `NOVANAS_PLUGIN_VERSION` | The version being installed. |
| `NOVANAS_PLUGIN_ROOT` | The on-disk plugin root. |

Hooks must be idempotent and complete in under 30 seconds. They are
not the place to do heavy work — use the runtime (helm release /
systemd unit) for that.

## Build & sign

The marketplace repo provides a `make plugin` target that:

1. Validates `manifest.yaml` against the v1 JSON schema.
2. Tars + gzips the package directory.
3. Signs with cosign:

```sh
cosign sign-blob --key cosign.key \
                 --output-signature rustfs-1.2.3.tar.gz.sig \
                 rustfs-1.2.3.tar.gz
```

4. Uploads both artifacts as a GitHub Release asset and updates
   `index.json`.

The cosign keypair is held by NovaNAS marketplace maintainers;
external authors submit a PR with the unsigned tarball + a manifest
PR, and a maintainer signs and tags the release.

## Test against a local nova-api

For development:

1. Set up a fake marketplace by serving a directory of fixtures over
   `python3 -m http.server`.

2. Generate a test cosign keypair:

```sh
cosign generate-key-pair
sudo cp cosign.pub /etc/nova-nas/trust/marketplace.pub
```

3. Build and sign your plugin:

```sh
tar -C myplugin -czf myplugin-0.1.0.tar.gz .
cosign sign-blob --key cosign.key \
                 --output-signature myplugin-0.1.0.tar.gz.sig \
                 myplugin-0.1.0.tar.gz
```

4. Publish locally:

```sh
mkdir -p www/releases/myplugin/0.1.0
cp myplugin-0.1.0.tar.gz www/releases/myplugin/0.1.0/
cp myplugin-0.1.0.tar.gz.sig www/releases/myplugin/0.1.0/

cat > www/index.json <<EOF
{
  "version": 1,
  "plugins": [
    {
      "name": "myplugin",
      "vendor": "Me",
      "category": "utility",
      "versions": [
        {
          "version": "0.1.0",
          "tarballUrl": "http://localhost:8000/releases/myplugin/0.1.0/myplugin-0.1.0.tar.gz",
          "signatureUrl": "http://localhost:8000/releases/myplugin/0.1.0/myplugin-0.1.0.tar.gz.sig"
        }
      ]
    }
  ]
}
EOF
(cd www && python3 -m http.server)
```

5. Point nova-api at it:

```sh
export MARKETPLACE_INDEX_URL=http://localhost:8000/index.json
sudo systemctl restart nova-api
```

6. Install:

```sh
curl -X POST -H "Authorization: Bearer $TOK" \
     -H "Content-Type: application/json" \
     -d '{"name":"myplugin","version":"0.1.0"}' \
     https://nas.local/api/v1/plugins
```

## Reference

- `internal/plugins/manifest.go` — the canonical schema; if the
  validator changes, this guide must change with it.
- `docs/plugins/README.md` — the operator-facing guide.
- The marketplace repo: https://github.com/azrtydxb/NovaNas-packages
