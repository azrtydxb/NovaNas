# Identity troubleshooting

Keycloak, OpenBao, external federation, token clock skew, SMB auth
failures.

## OpenBao unseal

**Symptom.** OpenBao pod is `Running` but health checks fail; the
operators log "vault sealed".

**Diagnose.**

```sh
kubectl -n novanas-system exec -it openbao-0 -- \
  openbao status

# If TPM auto-unseal is configured:
kubectl -n novanas-system logs openbao-0 | grep -i tpm
```

**Root causes.**

1. TPM device not exposed to the container (volume mount missing or
   device node permissions wrong after an OS update).
2. Auto-unseal seal config changed between reboots — seal wrapping
   key doesn't match the TPM key.
3. Shamir unseal: the pod was restarted and needs keys re-entered.

**Remediate.**

- TPM: verify `/dev/tpmrm0` is mounted into the pod and the
  serviceAccount has `/sys/class/tpm` access. Bounce the pod once the
  volume mount is corrected.
- Auto-unseal mismatch is rare and recoverable only if the original
  seal config is restored. If it is not, shamir unseal with the
  off-line keys:

  ```sh
  openbao operator unseal <key-1>
  openbao operator unseal <key-2>
  openbao operator unseal <key-3>
  ```

- See also [disaster-recovery.md](../runbook/disaster-recovery.md) for
  the full restore flow.

## Federation sync failure

<a id="federation-sync-failure"></a>

**Symptom.** An external-IdP user cannot log in despite "just" working
yesterday; local test users still work fine.

**Diagnose.**

```sh
kubectl -n novanas-system logs -l app=keycloak --tail=500 \
  | grep -iE 'user federation|ldap|saml'

# Verify the federation provider is healthy in Keycloak admin:
# Admin UI → User Federation → <provider> → Test connection
# CLI:
kcadm.sh get user-federation/ldap/<realm>/<id> -o
```

**Root causes.**

1. LDAP bind credential rotated upstream; Keycloak still has the old
   one.
2. TLS certificate on the LDAP server expired.
3. User was disabled in the upstream directory but Keycloak cached
   them as enabled for longer than the sync interval.
4. SAML IdP metadata rotated its signing cert.

**Remediate.**

- Update the bind credential in the Keycloak admin console or via
  `kcadm.sh`.
- Force a sync:

  ```sh
  kcadm.sh create user-federation/ldap/<id>/sync -s action=triggerFullSync
  ```

- For SAML, re-import the IdP metadata.

## SMB `NT_STATUS_LOGON_FAILURE`

<a id="smb-nt_status_logon_failure"></a>

**Symptom.** SMB mount fails; event log on the client shows
`NT_STATUS_LOGON_FAILURE` or `0xC000006D`.

**Diagnose.**

```sh
# On the SMB server pod:
kubectl -n novanas-system logs -l app=smbserver --tail=200 \
  | grep -iE 'auth|krb|logon'

# On the client:
klist                 # if Kerberos
smbstatus --users     # if shares are already mounted
```

**Root causes.**

1. Expired Kerberos ticket (most common on long-running sessions).
2. Clock skew > 5 min between the client and the NAS (Kerberos
   rejects).
3. User is in a Keycloak group that the SMB server does not map to a
   UID (check the `ShareUserMap`).
4. Password was rotated; cached credentials are stale.

**Remediate.**

- Re-kinit on the client.
- `chronyd` or `timedatectl set-ntp true` on both sides.
- Amend the share's `UserMap` or disable enforced group mapping.

## Token clock skew

<a id="token-clock-skew"></a>

**Symptom.** API returns `401 invalid token` for a token that just
worked; retry a minute later works.

**Diagnose.**

```sh
date -u                        # on the NovaNas node
date -u                        # on the client
# Diff > 60s is likely the cause.

kubectl -n novanas-system logs -l app=novanas-api --tail=200 \
  | grep -i 'jwt'
```

**Root causes.**

1. NTP is not running on one of the two sides.
2. The JWT was issued by a Keycloak replica whose clock drifted.

**Remediate.**

- Enable NTP both sides.
- If a Keycloak pod drifted, bounce it:

  ```sh
  kubectl -n novanas-system rollout restart deploy/keycloak
  ```

- As a stop-gap, bump the issuer's allowed clock skew in NovaNas API
  config from 30s to 120s. Do not leave it wide open.
