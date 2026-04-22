# AdGuard Home Helm Chart

Network-wide ads and trackers blocker.

## Install

```sh
helm install adguard-home oci://ghcr.io/azrtydxb/novanas-apps/adguard-home --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://adguard.com/adguard-home.html
