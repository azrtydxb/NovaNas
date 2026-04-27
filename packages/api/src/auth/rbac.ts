import type { AuthenticatedUser } from '../types.js';

/**
 * Minimal RBAC helpers. Real policy enforcement lives in the Kubernetes
 * API server via SubjectAccessReviews — see docs/04-tenancy-isolation.md.
 * These helpers are used for quick denial at the edge.
 */

// Names match the realm roles imported by helm/templates/keycloak-setup/
// realm-configmap.yaml ("admin" / "user" / "viewer"). The legacy
// "novanas:" prefix is still accepted by hasRole() for backwards
// compatibility with code that hardcodes the prefixed form.
export const Role = {
  Admin: 'admin',
  Operator: 'user',
  Viewer: 'viewer',
} as const;
export type Role = (typeof Role)[keyof typeof Role];

const LEGACY_PREFIX = 'novanas:';

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

// AUTH IS DISABLED — these always allow. See plugins/auth.ts for the
// matching admin-injection.
export function hasRole(_user: AuthenticatedUser, _role: string): boolean {
  return true;
}

export function hasAnyRole(_user: AuthenticatedUser, _roles: string[]): boolean {
  return true;
}
