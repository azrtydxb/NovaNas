# NovaNas LAN-discovery container.
#
# Bundles avahi-daemon (mDNS) so the appliance is browsable as
# `<cluster-name>.local`. The container runs in hostNetwork so its
# ads are visible on the LAN — there is no in-cluster discovery use
# case; this exists purely for client-side bookmarking.
#
# Configuration is rendered from environment variables at start time
# rather than baked in (NOVANAS_MDNS_DOMAIN / NOVANAS_CLUSTER_NAME
# come from the Helm values' global block).
FROM debian:bookworm-slim

RUN apt-get update \
 && apt-get install -y --no-install-recommends \
      avahi-daemon avahi-utils dbus-system-bus-common gettext-base \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /var/run/dbus /etc/avahi/services

# Avahi service template — populated at container start. We advertise
# https on 443 (the unified ingress) plus _smb._tcp / _nfs._tcp on
# their canonical ports so file-manager browse-by-mDNS works.
COPY avahi-novanas.service.tmpl /etc/avahi/services/novanas.service.tmpl

COPY entrypoint.sh /usr/local/bin/entrypoint.sh
RUN chmod +x /usr/local/bin/entrypoint.sh

ENTRYPOINT ["/usr/local/bin/entrypoint.sh"]
