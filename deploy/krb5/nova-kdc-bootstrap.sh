#!/bin/bash
# nova-kdc-bootstrap.sh — first-boot KDC initialization.
#
# Idempotent: if /var/lib/krb5kdc/principal already exists, this is a
# no-op. Otherwise it:
#   1. Renders /var/lib/krb5kdc/kdc.conf with the configured realm.
#   2. Creates the principal database with a stash file
#      (kdb5_util create -s -r REALM). The stash is written to a
#      transient persistent location (/var/lib/krb5kdc/.k5.<REALM>)
#      regardless of the runtime path in kdc.conf, because the create
#      flow needs to read it back to TPM-seal it.
#   3. If NOVA_KDC_TPM_SEAL=1 (the default for hardware with a TPM),
#      passes the plaintext stash through `nova-kdc-unseal --init` to
#      produce /etc/nova-kdc/master.enc, materializes the runtime stash
#      under /run/krb5kdc/, and shreds the persistent plaintext.
#      If NOVA_KDC_TPM_SEAL=0, the persistent plaintext stash is kept
#      and a fallback symlink/copy is placed at the runtime path; the
#      operator is warned that this path is a Tier-0 secret on disk.
#   4. Creates the bootstrap admin principal nova-kdc-admin/admin@REALM
#      and a host service principal host/<hostname>@REALM.
#
# Master key handling: TPM-sealing is the new default and mirrors the
# OpenBao TPM-unseal pattern (cmd/nova-bao-unseal). See
# docs/krb5/README.md for the full threat model.
#
# Environment (override in /etc/nova-kdc/bootstrap.env):
#   NOVA_KDC_REALM      — Kerberos realm (default NOVANAS.LOCAL)
#   NOVA_KDC_HOSTNAME   — short hostname for host/ principal (default $(hostname -f))
#   NOVA_KDC_MASTER_PW  — path to master-password file (default /etc/nova-kdc/master.pw)
#   NOVA_KDC_CONF_SRC   — path to kdc.conf template (default /usr/share/nova-nas/krb5/kdc.conf)
#   NOVA_KDC_ACL_SRC    — path to kadm5.acl template (default /usr/share/nova-nas/krb5/kadm5.acl)
#   NOVA_KDC_TPM_SEAL   — 1 (default): TPM-seal the master key; 0: keep
#                         plaintext stash on disk (fallback for hosts
#                         without a TPM).
#   NOVA_KDC_UNSEAL_BIN — path to nova-kdc-unseal binary
#                         (default /usr/local/bin/nova-kdc-unseal)
#   NOVA_KDC_SEALED_BLOB — TPM-sealed blob output path
#                          (default /etc/nova-kdc/master.enc)

set -euo pipefail

REALM="${NOVA_KDC_REALM:-NOVANAS.LOCAL}"
HOSTNAME_FQDN="${NOVA_KDC_HOSTNAME:-$(hostname -f 2>/dev/null || hostname)}"
MASTER_PW_FILE="${NOVA_KDC_MASTER_PW:-/etc/nova-kdc/master.pw}"
KDC_CONF_SRC="${NOVA_KDC_CONF_SRC:-/usr/share/nova-nas/krb5/kdc.conf}"
ACL_SRC="${NOVA_KDC_ACL_SRC:-/usr/share/nova-nas/krb5/kadm5.acl}"
TPM_SEAL="${NOVA_KDC_TPM_SEAL:-1}"
UNSEAL_BIN="${NOVA_KDC_UNSEAL_BIN:-/usr/local/bin/nova-kdc-unseal}"
SEALED_BLOB="${NOVA_KDC_SEALED_BLOB:-/etc/nova-kdc/master.enc}"

KDC_DIR=/var/lib/krb5kdc
KDC_CONF="$KDC_DIR/kdc.conf"
ACL_FILE="$KDC_DIR/kadm5.acl"
DB_FILE="$KDC_DIR/principal"

# The stash kdb5_util creates is the *persistent plaintext* — we hold
# it at this path only long enough to seal it, then shred. In non-TPM
# mode it is kept here as the long-term stash.
PERSIST_STASH="$KDC_DIR/.k5.${REALM}"

# Runtime tmpfs stash path (matches kdc.conf key_stash_file).
RUN_STASH_DIR=/run/krb5kdc
RUN_STASH="$RUN_STASH_DIR/.k5.${REALM}"

if [[ -f "$DB_FILE" ]]; then
    echo "nova-kdc-bootstrap: $DB_FILE already exists, nothing to do."
    exit 0
fi

if [[ ! -f "$MASTER_PW_FILE" ]]; then
    echo "nova-kdc-bootstrap: master password file $MASTER_PW_FILE missing." >&2
    echo "Generate one with: head -c 32 /dev/urandom | base64 > $MASTER_PW_FILE && chmod 600 $MASTER_PW_FILE" >&2
    exit 1
fi
chmod 600 "$MASTER_PW_FILE" || true

mkdir -p "$KDC_DIR"
chmod 700 "$KDC_DIR"
mkdir -p "$RUN_STASH_DIR"
chmod 700 "$RUN_STASH_DIR"

# Render kdc.conf with the configured realm. The shipped template uses
# NOVANAS.LOCAL as the placeholder; sed-substitute when the operator chose
# a different realm.
if [[ -f "$KDC_CONF_SRC" ]]; then
    sed "s/NOVANAS\\.LOCAL/${REALM}/g" "$KDC_CONF_SRC" > "$KDC_CONF"
    chmod 644 "$KDC_CONF"
fi
if [[ -f "$ACL_SRC" ]]; then
    sed "s/NOVANAS\\.LOCAL/${REALM}/g" "$ACL_SRC" > "$ACL_FILE"
    chmod 600 "$ACL_FILE"
fi

MASTER_PW="$(cat "$MASTER_PW_FILE")"
if [[ -z "$MASTER_PW" ]]; then
    echo "nova-kdc-bootstrap: master password file is empty" >&2
    exit 1
fi

echo "nova-kdc-bootstrap: creating database for realm $REALM..."
# -f forces the stash path to the persistent location regardless of
# the runtime path baked into kdc.conf — we need to read it back to
# seal (or to keep it as the long-term stash in non-TPM mode).
kdb5_util -r "$REALM" -P "$MASTER_PW" -f "$PERSIST_STASH" create -s

if [[ "$TPM_SEAL" == "1" ]]; then
    if [[ ! -x "$UNSEAL_BIN" ]]; then
        echo "nova-kdc-bootstrap: NOVA_KDC_TPM_SEAL=1 but $UNSEAL_BIN is not executable." >&2
        echo "Set NOVA_KDC_TPM_SEAL=0 to keep the plaintext stash, or install nova-kdc-unseal." >&2
        exit 1
    fi
    echo "nova-kdc-bootstrap: TPM-sealing master key to $SEALED_BLOB"
    install -d -m 700 "$(dirname "$SEALED_BLOB")"
    "$UNSEAL_BIN" --init --realm "$REALM" --blob "$SEALED_BLOB" --input "$PERSIST_STASH"

    # Materialize the runtime stash now so kadmin.local / krb5kdc work
    # this boot without waiting for nova-kdc-unseal.service.
    install -m 600 -o root -g root "$PERSIST_STASH" "$RUN_STASH"

    # Plaintext stash on persistent storage is the threat we just
    # eliminated — shred it. shred is a coreutils dependency on every
    # supported distro; if it's missing we fall back to overwrite + rm.
    if command -v shred >/dev/null 2>&1; then
        shred -u "$PERSIST_STASH"
    else
        dd if=/dev/urandom of="$PERSIST_STASH" bs=4096 count=1 conv=notrunc 2>/dev/null || true
        rm -f "$PERSIST_STASH"
    fi
    echo "nova-kdc-bootstrap: plaintext stash shredded; sealed blob is the only on-disk copy."
else
    echo "nova-kdc-bootstrap: NOVA_KDC_TPM_SEAL=0 — keeping plaintext stash at $PERSIST_STASH"
    echo "nova-kdc-bootstrap: WARNING: master key is on persistent storage (mode 0600 root:root)." >&2
    echo "nova-kdc-bootstrap: kdc.conf still points at $RUN_STASH; copying for this boot." >&2
    install -m 600 -o root -g root "$PERSIST_STASH" "$RUN_STASH"
    echo "nova-kdc-bootstrap: NOTE: on reboot the runtime stash will be missing. Either" >&2
    echo "  (a) edit /var/lib/krb5kdc/kdc.conf to set key_stash_file = $PERSIST_STASH, or" >&2
    echo "  (b) add a tmpfiles.d entry that re-copies the stash into $RUN_STASH_DIR at boot." >&2
fi

# Bootstrap admin principal — kadmind ACL grants this principal full rights.
# Random key; operators that want password login can rotate via kadmin.local.
kadmin.local -q "add_principal -randkey nova-kdc-admin/admin@${REALM}" >/dev/null
kadmin.local -q "ktadd -k /var/lib/krb5kdc/kadm5.keytab nova-kdc-admin/admin@${REALM}" >/dev/null

# Host service principal so the local NFS server can mount sec=krb5*.
kadmin.local -q "add_principal -randkey host/${HOSTNAME_FQDN}@${REALM}" >/dev/null
kadmin.local -q "ktadd -k /etc/krb5.keytab host/${HOSTNAME_FQDN}@${REALM}" >/dev/null

echo "nova-kdc-bootstrap: done. Realm=${REALM}, Host=${HOSTNAME_FQDN}, TPM_SEAL=${TPM_SEAL}"
