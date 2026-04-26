import type { AuthenticatedUser } from '../types.js';

/**
 * Edge authorization helpers. Real enforcement still happens inside the
 * Kubernetes API server via RBAC + operators; these checks short-circuit
 * forbidden requests at the Fastify layer.
 *
 * Roles (Keycloak realm roles):
 *   - `novanas:admin`       — full access to all resources, any namespace
 *   - `novanas:user`        — CRUD in their own namespace only
 *   - `novanas:viewer`      — read-only, all namespaces
 *   - `novanas:share-only`  — no API access (share clients)
 *
 * Group claim (`groups`) is used to scope the user to a namespace. The
 * user's "own namespace" is `user-<username>` by convention.
 */

// Names mirror the realm roles from the keycloak realm import. Both
// the bare names and the historical "novanas:" prefixed forms are
// recognised by hasAnyRole below.
export const AuthzRole = {
  Admin: 'admin',
  User: 'user',
  Viewer: 'viewer',
  ShareOnly: 'share-only',
} as const;
export type AuthzRole = (typeof AuthzRole)[keyof typeof AuthzRole];

export type Action = 'read' | 'write' | 'delete';

/** Resource kinds gate-kept by authz. */
export type Kind =
  | 'StoragePool'
  | 'Dataset'
  | 'Bucket'
  | 'Share'
  | 'Disk'
  | 'Snapshot'
  | 'User'
  | 'AppInstance'
  | 'ObjectStore'
  | 'BucketUser'
  | 'SmbServer'
  | 'NfsServer'
  | 'IscsiTarget'
  | 'NvmeofTarget'
  | 'AppCatalog'
  | 'App'
  | 'Vm'
  | 'IsoLibrary'
  // B2: crypto
  | 'EncryptionPolicy'
  | 'KmsKey'
  | 'Certificate'
  // B2: data protection
  | 'SnapshotSchedule'
  | 'ReplicationTarget'
  | 'ReplicationJob'
  | 'CloudBackupTarget'
  | 'CloudBackupJob'
  | 'ScrubSchedule'
  // B2: networking
  | 'Bond'
  | 'Vlan'
  | 'HostInterface'
  | 'ClusterNetwork'
  | 'VipPool'
  | 'Ingress'
  | 'RemoteAccessTunnel'
  | 'CustomDomain'
  | 'FirewallRule'
  | 'TrafficPolicy'
  | 'PhysicalInterface'
  // B2: ops
  | 'SmartPolicy'
  | 'AlertChannel'
  | 'AlertPolicy'
  | 'AuditPolicy'
  | 'UpsPolicy'
  | 'ServiceLevelObjective'
  | 'ConfigBackupPolicy'
  // B2: system singletons
  | 'SystemSettings'
  | 'UpdatePolicy'
  | 'ServicePolicy'
  // B2: identity
  | 'Group'
  | 'KeycloakRealm'
  | 'ApiToken'
  | 'SshKey'
  // B2: devices
  | 'GpuDevice';

/** Kinds that are cluster-scoped per our design. */
const CLUSTER_SCOPED: ReadonlySet<Kind> = new Set([
  'StoragePool',
  'Dataset',
  'Bucket',
  'Disk',
  'Snapshot',
  'User',
  'ObjectStore',
  'BucketUser',
  'SmbServer',
  'NfsServer',
  'IscsiTarget',
  'NvmeofTarget',
  'AppCatalog',
  'App',
  'IsoLibrary',
  // B2: all additional kinds are cluster-scoped
  'EncryptionPolicy',
  'KmsKey',
  'Certificate',
  'SnapshotSchedule',
  'ReplicationTarget',
  'ReplicationJob',
  'CloudBackupTarget',
  'CloudBackupJob',
  'ScrubSchedule',
  'Bond',
  'Vlan',
  'HostInterface',
  'ClusterNetwork',
  'VipPool',
  'Ingress',
  'RemoteAccessTunnel',
  'CustomDomain',
  'FirewallRule',
  'TrafficPolicy',
  'PhysicalInterface',
  'SmartPolicy',
  'AlertChannel',
  'AlertPolicy',
  'AuditPolicy',
  'UpsPolicy',
  'ServiceLevelObjective',
  'ConfigBackupPolicy',
  'SystemSettings',
  'UpdatePolicy',
  'ServicePolicy',
  'Group',
  'KeycloakRealm',
  'ApiToken',
  'SshKey',
  'GpuDevice',
]);

/** Kinds that only admins may mutate. */
const ADMIN_ONLY_WRITE: ReadonlySet<Kind> = new Set([
  'StoragePool',
  'Disk',
  'User',
  'SmbServer',
  'NfsServer',
  'IscsiTarget',
  'NvmeofTarget',
  'AppCatalog',
  'App',
  'IsoLibrary',
  // B2 admin-only-write (system-wide / cluster-critical)
  'EncryptionPolicy',
  'KmsKey',
  'Certificate',
  'SnapshotSchedule',
  'ScrubSchedule',
  'Bond',
  'Vlan',
  'HostInterface',
  'ClusterNetwork',
  'VipPool',
  'Ingress',
  'RemoteAccessTunnel',
  'CustomDomain',
  'FirewallRule',
  'TrafficPolicy',
  'PhysicalInterface',
  'SmartPolicy',
  'AlertChannel',
  'AlertPolicy',
  'AuditPolicy',
  'UpsPolicy',
  'ConfigBackupPolicy',
  'SystemSettings',
  'UpdatePolicy',
  'ServicePolicy',
  'KeycloakRealm',
  'GpuDevice',
]);

export function ownNamespace(user: AuthenticatedUser): string {
  return `user-${user.username}`;
}

export function isCluster(kind: Kind): boolean {
  return CLUSTER_SCOPED.has(kind);
}

function hasAnyRole(user: AuthenticatedUser, roles: readonly string[]): boolean {
  // Roles arrive in user.roles (from realm_access.roles) or
  // user.groups (from the keycloak novanas:roles mapper). Search both
  // and accept the legacy "novanas:" prefix for backwards compat.
  const haystack = [...(user.roles ?? []), ...(user.groups ?? [])];
  for (const r of roles) {
    const bare = r.startsWith('novanas:') ? r.slice('novanas:'.length) : r;
    const prefixed = `novanas:${bare}`;
    if (haystack.includes(bare) || haystack.includes(prefixed)) return true;
  }
  return false;
}

/**
 * Internal service accounts (disk-agent, storage-meta, etc.) carry
 * `internal:*` role names from `auth/tokenreview.ts`. They get full
 * read access on every kind, plus targeted write access on the kinds
 * they own:
 *   internal:disk-agent → write Disk
 *   internal:storage    → write StoragePool, Disk (status only)
 * Anything else falls through to the normal user/role check above.
 */
const INTERNAL_WRITE_KINDS: Record<string, ReadonlySet<Kind>> = {
  'internal:disk-agent': new Set(['Disk']),
  'internal:storage': new Set(['Disk', 'StoragePool']),
  'internal:operator': new Set([]),
};

function isInternalServiceAccount(user: AuthenticatedUser): boolean {
  return (user.roles ?? []).some((r) => r.startsWith('internal:'));
}

function internalCanAccess(user: AuthenticatedUser, action: Action, kind: Kind): boolean {
  if (action === 'read') return true;
  // Write or delete — only the explicitly-listed kinds.
  for (const role of user.roles ?? []) {
    const allowed = INTERNAL_WRITE_KINDS[role];
    if (allowed?.has(kind)) return true;
  }
  return false;
}

export function isShareOnly(user: AuthenticatedUser): boolean {
  return (
    hasAnyRole(user, [AuthzRole.ShareOnly]) &&
    !hasAnyRole(user, [AuthzRole.Admin, AuthzRole.User, AuthzRole.Viewer])
  );
}

function canAccess(
  user: AuthenticatedUser,
  action: Action,
  kind: Kind,
  namespace?: string
): boolean {
  if (!user || isShareOnly(user)) return false;

  // Service accounts (TokenReview-authenticated internal callers) take
  // a separate path with kind-targeted write grants.
  if (isInternalServiceAccount(user)) {
    return internalCanAccess(user, action, kind);
  }

  const admin = hasAnyRole(user, [AuthzRole.Admin]);
  const regular = hasAnyRole(user, [AuthzRole.User]);
  const viewer = hasAnyRole(user, [AuthzRole.Viewer]);

  if (admin) return true;

  if (action === 'read') {
    if (viewer || regular) return true;
    return false;
  }

  // write or delete
  if (!regular) return false;
  if (ADMIN_ONLY_WRITE.has(kind)) return false;
  if (isCluster(kind)) {
    // cluster-scoped kinds the user CAN touch (Dataset, Bucket, Snapshot)
    // are only writable in their own labelled scope; we can't express that
    // purely with authz at the edge, so accept and let Kubernetes RBAC enforce.
    return true;
  }
  // namespaced kind (AppInstance) — must match the user's own namespace
  if (!namespace) return false;
  return namespace === ownNamespace(user);
}

export function canRead(user: AuthenticatedUser, kind: Kind, namespace?: string): boolean {
  return canAccess(user, 'read', kind, namespace);
}

export function canWrite(user: AuthenticatedUser, kind: Kind, namespace?: string): boolean {
  return canAccess(user, 'write', kind, namespace);
}

export function canDelete(user: AuthenticatedUser, kind: Kind, namespace?: string): boolean {
  return canAccess(user, 'delete', kind, namespace);
}

/**
 * Action-level authorization check used by E1-API-Actions.
 *
 * Model:
 *  - Admin: any action on any resource.
 *  - User: actions on resources within their own namespace (namespaced kinds),
 *    or any action on user-writable cluster-scoped kinds.
 *  - Viewer / share-only: 403 on all actions.
 *
 * Destructive actions (`delete`) additionally require canDelete() to pass.
 */
export function canAction(
  user: AuthenticatedUser,
  kind: Kind,
  action: string,
  namespace?: string
): boolean {
  if (!user || isShareOnly(user)) return false;
  if (hasAnyRole(user, [AuthzRole.Admin])) return true;
  if (hasAnyRole(user, [AuthzRole.Viewer]) && !hasAnyRole(user, [AuthzRole.User])) return false;
  // destructive actions require delete rights
  if (action === 'delete' || action === 'destroy') {
    return canDelete(user, kind, namespace);
  }
  return canWrite(user, kind, namespace);
}
