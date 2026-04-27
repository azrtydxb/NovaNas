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
  | 'GpuDevice'
  // Storage internals (system-managed; agent + controller mediate)
  | 'BackendAssignment';

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
  'BackendAssignment',
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
  'BackendAssignment',
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

// AUTH IS DISABLED — every gate returns true so the route surface is
// wide open. See plugins/auth.ts for the synthetic-admin injection
// that pairs with this, and the GitHub issue tracking re-enablement.
export function canRead(_u: AuthenticatedUser, _k: Kind, _ns?: string): boolean {
  return true;
}

export function canWrite(_u: AuthenticatedUser, _k: Kind, _ns?: string): boolean {
  return true;
}

export function canDelete(_u: AuthenticatedUser, _k: Kind, _ns?: string): boolean {
  return true;
}

export function canAction(_u: AuthenticatedUser, _k: Kind, _action: string, _ns?: string): boolean {
  return true;
}
