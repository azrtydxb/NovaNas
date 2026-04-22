# Zigbee2MQTT Helm Chart

Zigbee to MQTT bridge.

## Install

```sh
helm install zigbee2mqtt oci://ghcr.io/azrtydxb/novanas-apps/zigbee2mqtt --version 0.1.0
```

## Values

See `values.yaml`. Common overrides:

- `image.tag` — pin container version
- `persistence.config.size` — config volume size
- `ingress.host` — FQDN for NovaEdge ingress
- `resources` — requests/limits

## Upstream

https://www.zigbee2mqtt.io
