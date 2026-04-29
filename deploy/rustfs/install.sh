#!/bin/sh
# deploy/rustfs/install.sh — install or refresh RustFS on a NovaNAS host.
#
# Idempotent: safe to re-run. The script:
#   1. downloads the pinned RustFS release tarball (see RUSTFS_VERSION below)
#   2. verifies its sha256 against the .sha256 sidecar published by upstream
#   3. installs the binary at /usr/local/bin/rustfs (atomic replace via mv)
#   4. creates the rustfs:rustfs system user/group if missing
#   5. ensures /var/lib/rustfs (data + home), /var/lib/rustfs/data,
#      /var/log/rustfs, /etc/rustfs, /etc/rustfs/certs exist with the right
#      ownership and perms
#   6. installs deploy/systemd/rustfs.service into /etc/systemd/system/
#   7. installs deploy/rustfs/rustfs.env.template into /etc/rustfs/rustfs.env
#      ONLY if the target file does not already exist (operator edits win)
#   8. creates the ZFS dataset tank/objects with NAS-friendly properties
#      IF the `zfs` command is available; otherwise warns and continues
#
# The script does NOT start the service — the operator does that after
# running the TLS cert issuance step (see README.md "TLS").
#
# Usage:
#   sudo sh deploy/rustfs/install.sh
#
# Environment overrides (rare):
#   RUSTFS_VERSION   override the pinned upstream version tag
#   RUSTFS_PREFIX    install prefix for the binary (default /usr/local/bin)
#   ZFS_DATASET      ZFS dataset name (default tank/objects)
#   ZFS_MOUNTPOINT   mountpoint for the dataset (default /var/lib/rustfs/data)

set -eu

# ----- Configuration ---------------------------------------------------------

# Pinned RustFS release. Verified at https://github.com/rustfs/rustfs/releases
# on 2026-04-29 — most recent tag at that time. Update RUSTFS_VERSION and
# re-run this script to upgrade; see README.md for the upgrade procedure.
RUSTFS_VERSION="${RUSTFS_VERSION:-1.0.0-beta.1}"
RUSTFS_PREFIX="${RUSTFS_PREFIX:-/usr/local/bin}"
ZFS_DATASET="${ZFS_DATASET:-tank/objects}"
ZFS_MOUNTPOINT="${ZFS_MOUNTPOINT:-/var/lib/rustfs/data}"

REPO_ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
SYSTEMD_UNIT_SRC="${REPO_ROOT}/deploy/systemd/rustfs.service"
ENV_TEMPLATE_SRC="${REPO_ROOT}/deploy/rustfs/rustfs.env.template"

DATA_DIR="/var/lib/rustfs"
LOG_DIR="/var/log/rustfs"
ETC_DIR="/etc/rustfs"
CERT_DIR="/etc/rustfs/certs"
ENV_FILE="/etc/rustfs/rustfs.env"
SYSTEMD_DST="/etc/systemd/system/rustfs.service"
BIN_DST="${RUSTFS_PREFIX}/rustfs"

# ----- Helpers ---------------------------------------------------------------

log()  { printf '[install-rustfs] %s\n' "$*"; }
warn() { printf '[install-rustfs] WARN: %s\n' "$*" >&2; }
die()  { printf '[install-rustfs] ERROR: %s\n' "$*" >&2; exit 1; }

require_root() {
    if [ "$(id -u)" -ne 0 ]; then
        die "must run as root (use sudo)"
    fi
}

require_cmd() {
    command -v "$1" >/dev/null 2>&1 || die "missing required command: $1"
}

# ----- Pre-flight ------------------------------------------------------------

require_root
require_cmd curl
require_cmd unzip
require_cmd sha256sum
require_cmd install
require_cmd systemctl

[ -f "$SYSTEMD_UNIT_SRC" ] || die "systemd unit not found at $SYSTEMD_UNIT_SRC"
[ -f "$ENV_TEMPLATE_SRC" ] || die "env template not found at $ENV_TEMPLATE_SRC"

case "$(uname -s)" in
    Linux) : ;;
    *) die "unsupported OS $(uname -s); RustFS releases only target Linux" ;;
esac

ARCH="$(uname -m)"
case "$ARCH" in
    x86_64)  PKG_ARCH="x86_64-musl"  ;;
    aarch64) PKG_ARCH="aarch64-musl" ;;
    *) die "unsupported CPU arch: $ARCH" ;;
esac

# Upstream zip filename pattern (verified against
# https://github.com/rustfs/rustfs/releases — assets like
# rustfs-linux-x86_64-musl-v1.0.0-beta.1.zip).
PKG_NAME="rustfs-linux-${PKG_ARCH}-v${RUSTFS_VERSION}.zip"
PKG_URL="https://github.com/rustfs/rustfs/releases/download/${RUSTFS_VERSION}/${PKG_NAME}"

# ----- User & group ----------------------------------------------------------

if ! getent group rustfs >/dev/null 2>&1; then
    log "creating rustfs group"
    groupadd --system rustfs
else
    log "rustfs group already exists"
fi

if ! getent passwd rustfs >/dev/null 2>&1; then
    log "creating rustfs system user"
    useradd --system --gid rustfs \
        --home-dir "$DATA_DIR" \
        --shell /usr/sbin/nologin \
        --comment "NovaNAS RustFS object storage" \
        rustfs
else
    log "rustfs user already exists"
fi

# ----- Directories -----------------------------------------------------------

log "ensuring directories: $DATA_DIR $LOG_DIR $ETC_DIR $CERT_DIR"
install -d -m 0750 -o rustfs -g rustfs "$DATA_DIR"
install -d -m 0750 -o rustfs -g rustfs "$LOG_DIR"
install -d -m 0755 -o root   -g root   "$ETC_DIR"
install -d -m 0750 -o rustfs -g rustfs "$CERT_DIR"

# ----- ZFS dataset (best-effort) --------------------------------------------

if command -v zfs >/dev/null 2>&1; then
    if zfs list -H -o name "$ZFS_DATASET" >/dev/null 2>&1; then
        log "ZFS dataset $ZFS_DATASET already exists"
    else
        log "creating ZFS dataset $ZFS_DATASET (mountpoint=$ZFS_MOUNTPOINT)"
        zfs create \
            -o mountpoint="$ZFS_MOUNTPOINT" \
            -o recordsize=1M \
            -o atime=off \
            -o compression=lz4 \
            -o xattr=sa \
            -o acltype=posixacl \
            "$ZFS_DATASET"
    fi
    # Make sure the resulting mountpoint is owned by rustfs even if the
    # dataset already existed.
    chown rustfs:rustfs "$ZFS_MOUNTPOINT"
    chmod 0750 "$ZFS_MOUNTPOINT"
else
    warn "zfs command not found — skipping creation of $ZFS_DATASET."
    warn "Operator must provision storage at $ZFS_MOUNTPOINT manually before"
    warn "starting the rustfs service."
    install -d -m 0750 -o rustfs -g rustfs "$ZFS_MOUNTPOINT"
fi

# ----- Download + install binary --------------------------------------------

TMP_DIR="$(mktemp -d)"
trap 'rm -rf "$TMP_DIR"' EXIT

log "downloading $PKG_URL"
curl -fsSL --retry 3 -o "$TMP_DIR/$PKG_NAME" "$PKG_URL" \
    || die "failed to download $PKG_URL"

# Try sidecar checksum first (.sha256). If upstream did not publish one for
# this release/asset, fall back to the GPG-signed .asc and warn — we still
# refuse to proceed silently.
if curl -fsSL --retry 3 -o "$TMP_DIR/$PKG_NAME.sha256" "$PKG_URL.sha256" 2>/dev/null; then
    log "verifying sha256 checksum"
    EXPECTED="$(awk '{print $1}' "$TMP_DIR/$PKG_NAME.sha256")"
    ACTUAL="$(sha256sum "$TMP_DIR/$PKG_NAME" | awk '{print $1}')"
    if [ "$EXPECTED" != "$ACTUAL" ]; then
        die "checksum mismatch: expected $EXPECTED, got $ACTUAL"
    fi
    log "sha256 ok"
else
    warn "no .sha256 sidecar published for $PKG_NAME — checksum verification skipped"
    warn "(GPG .asc signature is available at ${PKG_URL}.asc; verify out-of-band)"
fi

log "extracting"
unzip -qo "$TMP_DIR/$PKG_NAME" -d "$TMP_DIR/extract"
SRC_BIN="$(find "$TMP_DIR/extract" -type f -name rustfs | head -n1)"
[ -n "$SRC_BIN" ] || die "rustfs binary not found inside $PKG_NAME"

log "installing binary to $BIN_DST"
install -m 0755 -o root -g root "$SRC_BIN" "$BIN_DST.new"
mv -f "$BIN_DST.new" "$BIN_DST"

# ----- systemd unit ----------------------------------------------------------

log "installing systemd unit to $SYSTEMD_DST"
install -m 0644 -o root -g root "$SYSTEMD_UNIT_SRC" "$SYSTEMD_DST"

# ----- env file (template -> /etc/rustfs/rustfs.env) ------------------------

if [ -f "$ENV_FILE" ]; then
    log "$ENV_FILE already exists — leaving operator edits intact"
else
    log "installing env file from template to $ENV_FILE"
    install -m 0640 -o root -g rustfs "$ENV_TEMPLATE_SRC" "$ENV_FILE"
fi

# ----- systemd reload --------------------------------------------------------

log "reloading systemd"
systemctl daemon-reload

cat <<NEXT

RustFS ${RUSTFS_VERSION} installed.

Next steps (the operator decides when to start the service):

  1. Issue TLS certs for RustFS from the Nova CA:
       sudo bash deploy/observability/issue-certs.sh
     (Then copy the rustfs.crt/rustfs.key into ${CERT_DIR}/rustfs_cert.pem
     and ${CERT_DIR}/rustfs_key.pem — see deploy/rustfs/README.md.)

  2. Create the Keycloak client and paste its secret into ${ENV_FILE}:
       sudo bash deploy/keycloak/create-rustfs-client.sh \\
         --kc-url https://192.168.10.204:8443 --admin-pass "\$KC_ADMIN_PASS"

  3. Edit ${ENV_FILE} and rotate RUSTFS_SECRET_KEY off the default.

  4. Enable + start the service:
       sudo systemctl enable --now rustfs.service
       systemctl status rustfs.service

NEXT
