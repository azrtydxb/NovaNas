# code-server Helm Chart

VS Code in the browser.

## Install

```sh
helm install code-server oci://ghcr.io/azrtydxb/novanas-apps/code-server --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://github.com/coder/code-server
