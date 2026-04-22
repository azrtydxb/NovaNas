# File Browser Helm Chart

Web file browser with markdown editor.

## Install

```sh
helm install filebrowser oci://ghcr.io/azrtydxb/novanas-apps/filebrowser --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://filebrowser.org
