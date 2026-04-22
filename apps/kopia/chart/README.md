# Kopia Helm Chart

Fast and secure open-source backup tool.

## Install

```sh
helm install kopia oci://ghcr.io/azrtydxb/novanas-apps/kopia --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://kopia.io
