# Home Assistant Helm Chart

Open source home automation.

## Install

```sh
helm install home-assistant oci://ghcr.io/azrtydxb/novanas-apps/home-assistant --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://www.home-assistant.io
