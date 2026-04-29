#!/bin/bash
# nova-kdc-bootstrap.sh — first-boot KDC initialization.
#
# Idempotent: if /var/lib/krb5kdc/principal already exists, this is a
# no-op. Otherwise it:
#   1. Renders /var/lib/krb5kdc/kdc.conf with the configured realm.
#   2. Creates the principal database with a stash file
#      (kdb5_util create -s -r REALM).
#   3. Creates the bootstrap admin principal nova-kdc-admin/admin@REALM
#      and a host service principal host/<hostname>@REALM.
#
# Master key handling: TPM-sealing the master key is on the roadmap
# (see docs/krb5/README.md and the openbao TPM unseal pattern in
# cmd/nova-bao-unseal). For v1 we use a stash file at
# /var/lib/krb5kdc/.k5.<REALM> with mode 0600 owned by root. The
# master password used to derive the stash is read from
# /etc/nova-kdc/master.pw (mode 0600) — operators should generate one
# at install time and back it up out-of-band.
#
# Environment (override in /etc/nova-kdc/bootstrap.env):
#   NOVA_KDC_REALM      — Kerberos realm (default NOVANAS.LOCAL)
#   NOVA_KDC_HOSTNAME   — short hostname for host/ principal (default $(hostname -f))
#   NOVA_KDC_MASTER_PW  — path to master-password file (default /etc/nova-kdc/master.pw)
#   NOVA_KDC_CONF_SRC   — path to kdc.conf template (default /usr/share/nova-nas/krb5/kdc.conf)
#   NOVA_KDC_ACL_SRC    — path to kadm5.acl template (default /usr/share/nova-nas/krb5/kadm5.acl)

set -euo pipefail

REALM="${NOVA_KDC_REALM:-NOVANAS.LOCAL}"
HOSTNAME_FQDN="${NOVA_KDC_HOSTNAME:-$(hostname -f 2>/dev/null || hostname)}"
MASTER_PW_FILE="${NOVA_KDC_MASTER_PW:-/etc/nova-kdc/master.pw}"
KDC_CONF_SRC="${NOVA_KDC_CONF_SRC:-/usr/share/nova-nas/krb5/kdc.conf}"
ACL_SRC="${NOVA_KDC_ACL_SRC:-/usr/share/nova-nas/krb5/kadm5.acl}"

KDC_DIR=/var/lib/krb5kdc
KDC_CONF="$KDC_DIR/kdc.conf"
ACL_FILE="$KDC_DIR/kadm5.acl"
DB_FILE="$KDC_DIR/principal"

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
kdb5_util -r "$REALM" -P "$MASTER_PW" create -s

# Bootstrap admin principal — kadmind ACL grants this principal full rights.
# Random key; operators that want password login can rotate via kadmin.local.
kadmin.local -q "add_principal -randkey nova-kdc-admin/admin@${REALM}" >/dev/null
kadmin.local -q "ktadd -k /var/lib/krb5kdc/kadm5.keytab nova-kdc-admin/admin@${REALM}" >/dev/null

# Host service principal so the local NFS server can mount sec=krb5*.
kadmin.local -q "add_principal -randkey host/${HOSTNAME_FQDN}@${REALM}" >/dev/null
kadmin.local -q "ktadd -k /etc/krb5.keytab host/${HOSTNAME_FQDN}@${REALM}" >/dev/null

echo "nova-kdc-bootstrap: done. Realm=${REALM}, Host=${HOSTNAME_FQDN}"
