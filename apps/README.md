# NovaNas App Catalog

The official NovaNas application catalog. Each entry is a Helm chart plus
NovaNas-specific metadata and is published as a signed OCI artifact consumed
by the NovaNas operator on appliances.

## Layout

```
apps/
├── README.md                  # this file
├── index.yaml                 # consolidated catalog index (consumed by AppCatalog CR)
├── scripts/
│   ├── lint-all.sh            # helm lint every chart
│   ├── sign-all.sh            # cosign-sign every published artifact
│   └── publish.sh             # helm push every chart to OCI
└── <app>/
    ├── chart/                 # Helm chart (Chart.yaml, values.yaml, templates/)
    ├── metadata.yaml          # NovaNas AppMetadata (category, icon, schema, ...)
    ├── icon.svg               # 28x28 icon (or icon.png)
    └── tests/smoke.sh         # post-install smoke probe
```

## Current catalog (36 apps)

| Category       | Apps                                                        |
|----------------|-------------------------------------------------------------|
| media          | plex, jellyfin, emby                                        |
| *arr           | sonarr, radarr, prowlarr, lidarr, readarr, bazarr, qbittorrent |
| photos         | immich, photoprism                                          |
| files          | nextcloud, seafile, filebrowser                             |
| home           | home-assistant, frigate, zigbee2mqtt                        |
| dev            | gitea, woodpecker, code-server                              |
| database       | postgres, mysql, mariadb, redis, mongodb                    |
| observability  | prometheus, grafana, loki                                   |
| utility        | vaultwarden, paperless-ngx                                  |
| networking     | adguard-home, pihole, nginx-proxy-manager                   |
| backup         | duplicati, kopia                                            |

## Contributing a new app

1. **Create the directory.**

   ```
   apps/<name>/
   ├── chart/                 # Helm chart
   ├── metadata.yaml
   ├── icon.svg
   └── tests/smoke.sh
   ```

2. **metadata.yaml.** Follow the schema defined in `docs/08-apps-and-vms.md`:

   - `apiVersion: novanas.io/v1alpha1`
   - `kind: AppMetadata`
   - `name`, `displayName`, `version`, `appVersion`, `category`
   - `description` (short), `longDescription` (paragraph)
   - `icon` (path, usually `icon.svg`)
   - `requirements` (minRamMB, minCpu, requiresGpu, ports)
   - `schema` — JSON Schema of user-tunable values surfaced in the UI

3. **Chart.** Keep it lean: one Deployment, one Service, one PVC, one Ingress.
   Use the `_helpers.tpl` conventions: `app.name`, `app.fullname`, `app.labels`,
   `app.selectorLabels`. Default the ingress host to `<app>.nas.local` and use
   the `novaedge` ingress class.

4. **Icon.** Prefer a real upstream SVG/PNG. Otherwise a 28x28 flat-color
   placeholder with the first letter of the app name is acceptable.

5. **Smoke test.** A shell script that, given `NAMESPACE` and `RELEASE`, waits
   for the rollout then probes the Service over HTTP or TCP. Exit 0 on
   success.

6. **Lint locally.**

   ```sh
   ./apps/scripts/lint-all.sh
   ```

7. **Add to `index.yaml`.** Append an entry with `name`, `version`, `category`,
   `displayName`, `icon`, `chartPath`, `metadataPath`, `ociRef`. The index is
   hand-maintained for now; a generator will come later.

## Release flow

The catalog has its own release cadence, independent from the appliance.

- **stable** — reviewed, smoke-tested, signed
- **beta** — latest upstream release, smoke-tested
- **dev** — nightly, unsigned

Pipeline (see `docs/13-build-and-release.md`):

1. `scripts/lint-all.sh` runs in CI on every PR.
2. On tag, `scripts/publish.sh` pushes each chart to
   `oci://ghcr.io/azrtydxb/novanas-apps/<name>:<version>`.
3. `scripts/sign-all.sh` cosign-signs every pushed artifact (keyless OIDC in
   CI, key-file for release managers).

## Consumption flow

Appliances consume the catalog via the NovaNas operator:

1. `AppCatalog` CR points at `ghcr.io/azrtydxb/novanas-apps` and a channel.
2. The operator polls `index.yaml` (published alongside the OCI repo) and
   reconciles a `CatalogApp` CR per entry.
3. Users browse the catalog in the NovaNas UI; installing an app creates an
   `AppInstance` CR that the operator renders by pulling
   `oci://ghcr.io/azrtydxb/novanas-apps/<name>:<version>` and applying it with
   the user-supplied values.
4. Every pulled chart is verified against its cosign signature before it is
   installed.

## Non-goals

- Full feature parity with upstream's "official" Helm chart — charts here are
  deliberately minimal. Power users can still swap in their own chart.
- E2E tests per app — the smoke probe is enough. Appliance-level e2e is
  handled in `e2e/`.
