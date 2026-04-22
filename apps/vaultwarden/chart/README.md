# Vaultwarden Helm Chart

Unofficial Bitwarden compatible server.

## Install

```sh
helm install vaultwarden oci://ghcr.io/azrtydxb/novanas-apps/vaultwarden --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://github.com/dani-garcia/vaultwarden
