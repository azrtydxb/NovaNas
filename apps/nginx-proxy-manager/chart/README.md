# Nginx Proxy Manager Helm Chart

Docker container for managing Nginx proxy hosts.

## Install

```sh
helm install nginx-proxy-manager oci://ghcr.io/azrtydxb/novanas-apps/nginx-proxy-manager --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://nginxproxymanager.com
