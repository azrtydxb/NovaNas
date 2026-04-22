# Gitea Helm Chart

Self-hosted Git service.

## Install

```sh
helm install gitea oci://ghcr.io/azrtydxb/novanas-apps/gitea --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://gitea.io
