# PhotoPrism Helm Chart

AI-powered app for browsing and sharing photos.

## Install

```sh
helm install photoprism oci://ghcr.io/azrtydxb/novanas-apps/photoprism --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://photoprism.app
