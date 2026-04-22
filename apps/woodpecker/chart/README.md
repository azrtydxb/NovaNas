# Woodpecker CI Helm Chart

Community-driven CI engine.

## Install

```sh
helm install woodpecker oci://ghcr.io/azrtydxb/novanas-apps/woodpecker --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://woodpecker-ci.org
