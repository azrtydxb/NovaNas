#!/bin/sh
set -eu

: "${NOVANAS_CLUSTER_NAME:=novanas}"
: "${NOVANAS_MDNS_DOMAIN:=local}"

# Avahi expects exactly one service file per advertised group. The
# template is checked into the image; we rewrite it at start-time so
# the cluster name reflects the current Helm values.
envsubst < /etc/avahi/services/novanas.service.tmpl \
        > /etc/avahi/services/novanas.service

# Avahi runs on the system D-Bus. The dbus daemon must be up first.
mkdir -p /var/run/dbus
dbus-daemon --system --fork

# host-name=$NOVANAS_CLUSTER_NAME so the appliance appears as
# <cluster>.local on every LAN client.
exec avahi-daemon \
  --no-drop-root \
  --no-rlimits \
  --no-chroot \
  --debug
