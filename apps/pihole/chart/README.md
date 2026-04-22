# Pi-hole Helm Chart

Network-wide ad blocker.

## Install

```sh
helm install pihole oci://ghcr.io/azrtydxb/novanas-apps/pihole --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://pi-hole.net
