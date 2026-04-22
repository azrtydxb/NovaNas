import type { AuthenticatedUser } from '../types.js';

/**
 * Minimal RBAC helpers. Real policy enforcement lives in the Kubernetes
 * API server via SubjectAccessReviews — see docs/04-tenancy-isolation.md.
 * These helpers are used for quick denial at the edge.
 */

export const Role = {
  Admin: 'novanas:admin',
  Operator: 'novanas:operator',
  Viewer: 'novanas:viewer',
} as const;
export type Role = (typeof Role)[keyof typeof Role];

export function userFromClaims(claims: Record<string, unknown>): AuthenticatedUser {
  const rolesFromRealmAccess = Array.isArray(
    (claims.realm_access as { roles?: unknown } | undefined)?.roles
  )
    ? ((claims.realm_access as { roles: unknown[] }).roles as string[])
    : [];

  const groups = Array.isArray(claims.groups) ? (claims.groups as string[]) : [];

  return {
    sub: String(claims.sub ?? ''),
    username: String(claims.preferred_username ?? claims.sub ?? ''),
    email: typeof claims.email === 'string' ? claims.email : undefined,
    name: typeof claims.name === 'string' ? claims.name : undefined,
    groups,
    roles: rolesFromRealmAccess,
    tenant: 'default',
    claims,
  };
}

export function hasRole(user: AuthenticatedUser, role: Role): boolean {
  return user.roles.includes(role);
}

export function hasAnyRole(user: AuthenticatedUser, roles: Role[]): boolean {
  return roles.some((r) => hasRole(user, r));
}
