# Frigate NVR Helm Chart

NVR with real-time local object detection.

## Install

```sh
helm install frigate oci://ghcr.io/azrtydxb/novanas-apps/frigate --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://frigate.video
