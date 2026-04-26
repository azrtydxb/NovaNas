# ADR 0009: Defer kubevirt / novanet / novaedge integrations

Status: accepted — 2026-04-26
Tracking: GH #41

## Context

The umbrella helm chart previously pulled three custom charts from
`oci://ghcr.io/azrtydxb/charts`:

- **kubevirt** — VM CRDs + virt-operator. The upstream
  `kubevirt-operator` manifest set is large; re-implementing it as
  in-house templates would be multi-day work that duplicates
  upstream effort with no NovaNas-specific value-add.
- **novanet** — custom L2/L3 controller. The controller binary itself
  does not exist in this repo; only the chart wiring did. A from-
  scratch controller is a separate green-field effort.
- **novaedge** — custom Ingress controller. Same situation as
  novanet — chart-without-controller.

Per the "we own all our charts" direction we stripped these from
`helm/Chart.yaml` (`dependencies: []`). The legacy values blocks
remained but were dead.

## Decision

For all three: **option 3 — drop the integration from this cycle,
gate the feature behind `<name>.enabled: false` by default, point
back to this ADR.**

Specifically:

- kubevirt — the VM lifecycle UX in the SPA continues to render
  read-only "kubevirt unavailable" placeholders when the CRD is
  absent. When a real KubeVirt deployment is required, the operator
  installs upstream `kubevirt-operator` separately as a sibling
  release; the umbrella chart does not own its lifecycle.
- novanet — overlay-network features are deferred. The values block
  is kept for future re-enable but `enabled: false` by default.
- novaedge — VIP-pool / advanced-ingress features stay deferred.
  The unified ingress (templates/ingress/) covers the appliance
  use case via traefik / nginx; novaedge would be needed only for
  multi-tenant edge routing.

## Consequences

- The umbrella chart installs cleanly without any of these.
- Features that depended on them (live VM migration, encrypted
  overlay, VIP pools) are gated and surface "feature unavailable"
  in the UI rather than failing.
- Re-enabling any of these is a future ADR + chart work.
