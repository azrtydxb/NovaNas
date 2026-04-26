// Minimal Keycloak Admin REST API client.
//
// Authenticates via the novanas-api confidential client's service
// account (client_credentials grant) and exposes the subset of admin
// operations the api needs to inline operator side effects (#51).
//
// Tokens are cached per-realm with a small skew; a 401 from Keycloak
// invalidates the cache so the next call re-fetches.

import type { Env } from '../env.js';

interface CachedToken {
  accessToken: string;
  expiresAt: number; // epoch ms
}

export interface KeycloakGroupSpec {
  name: string;
  realm?: string;
  members?: string[];
}

export interface KeycloakUserSpec {
  username: string;
  realm?: string;
  email?: string;
  firstName?: string;
  lastName?: string;
  enabled?: boolean;
  groups?: string[];
}

export interface KeycloakRealmUpdate {
  displayName?: string;
  enabled?: boolean;
  passwordPolicy?: string;
  defaultLocale?: string;
}

export interface KeycloakAdmin {
  /** Idempotent group ensure. Returns the Keycloak group id. */
  ensureGroup(spec: KeycloakGroupSpec): Promise<string>;
  /** Best-effort group delete. Missing group is not an error. */
  deleteGroup(realm: string, name: string): Promise<void>;
  /**
   * Update mutable fields on an existing realm. Realm CREATION
   * requires master-realm credentials and is owned by the post-install
   * Job; this is for runtime updates only. Returns true if the realm
   * existed and was updated, false if it was missing (404).
   */
  updateRealm(name: string, update: KeycloakRealmUpdate): Promise<boolean>;
  /** Idempotent user ensure. Returns the Keycloak user id. */
  ensureUser(spec: KeycloakUserSpec): Promise<string>;
  /** Best-effort user delete. Missing user is not an error. */
  deleteUser(realm: string, username: string): Promise<void>;
}

export function createKeycloakAdmin(env: Env): KeycloakAdmin {
  const baseUrl = (env.KEYCLOAK_INTERNAL_ISSUER_URL ?? env.KEYCLOAK_ISSUER_URL).replace(
    /\/realms\/[^/]+\/?$/,
    ''
  );
  const defaultRealm = inferDefaultRealm(env.KEYCLOAK_ISSUER_URL);
  const tokens = new Map<string, CachedToken>();

  async function getToken(realm: string): Promise<string> {
    const cached = tokens.get(realm);
    if (cached && cached.expiresAt > Date.now() + 5_000) return cached.accessToken;

    // Service-account tokens are issued by the realm hosting the
    // confidential client. The novanas-api client lives in the novanas
    // realm, so token issuance and admin calls both target that realm.
    const tokenUrl = `${baseUrl}/realms/${realm}/protocol/openid-connect/token`;
    const body = new URLSearchParams({
      grant_type: 'client_credentials',
      client_id: env.KEYCLOAK_CLIENT_ID,
      client_secret: env.KEYCLOAK_CLIENT_SECRET,
    });
    const res = await fetch(tokenUrl, {
      method: 'POST',
      headers: { 'content-type': 'application/x-www-form-urlencoded' },
      body: body.toString(),
    });
    if (!res.ok) {
      throw new Error(`keycloak-admin: token exchange failed (${res.status}): ${await res.text()}`);
    }
    const data = (await res.json()) as { access_token: string; expires_in: number };
    const t: CachedToken = {
      accessToken: data.access_token,
      expiresAt: Date.now() + data.expires_in * 1000,
    };
    tokens.set(realm, t);
    return t.accessToken;
  }

  async function adminFetch(
    realm: string,
    path: string,
    init: RequestInit = {}
  ): Promise<Response> {
    // The admin path may target a non-default realm (e.g. realm
    // management endpoints), but the bearer token always comes from
    // the SA's realm — Keycloak issues service-account tokens from the
    // realm hosting the confidential client, regardless of what realm
    // the request later operates on. With the realm-management roles
    // we grant, the SA can only manage its own realm anyway; ops on a
    // different realm simply 403, which the caller surfaces.
    const url = `${baseUrl}/admin/realms/${realm}${path}`;
    let token = await getToken(defaultRealm);
    let res = await fetch(url, {
      ...init,
      headers: { ...(init.headers ?? {}), authorization: `Bearer ${token}` },
    });
    if (res.status === 401) {
      tokens.delete(defaultRealm);
      token = await getToken(defaultRealm);
      res = await fetch(url, {
        ...init,
        headers: { ...(init.headers ?? {}), authorization: `Bearer ${token}` },
      });
    }
    return res;
  }

  return {
    async ensureGroup(spec: KeycloakGroupSpec): Promise<string> {
      const realm = spec.realm ?? defaultRealm;

      // Look up existing first; Keycloak's `/groups` listing supports
      // an exact match via search=...&exact=true.
      const lookup = await adminFetch(
        realm,
        `/groups?search=${encodeURIComponent(spec.name)}&exact=true`
      );
      if (lookup.ok) {
        const groups = (await lookup.json()) as Array<{ id: string; name: string }>;
        const found = groups.find((g) => g.name === spec.name);
        if (found) return found.id;
      }

      // Create. Keycloak's POST /groups returns 201 with Location:
      // /admin/realms/<r>/groups/<id>. Parse the id from there.
      const create = await adminFetch(realm, '/groups', {
        method: 'POST',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify({ name: spec.name }),
      });
      if (create.status === 409) {
        // Race: another caller created it. Re-lookup.
        const retry = await adminFetch(
          realm,
          `/groups?search=${encodeURIComponent(spec.name)}&exact=true`
        );
        const groups = (await retry.json()) as Array<{ id: string; name: string }>;
        const found = groups.find((g) => g.name === spec.name);
        if (found) return found.id;
      }
      if (!create.ok) {
        throw new Error(
          `keycloak-admin: ensureGroup ${spec.name} failed (${create.status}): ${await create.text()}`
        );
      }
      const location = create.headers.get('location') ?? '';
      const id = location.split('/').pop() ?? '';
      if (!id) throw new Error('keycloak-admin: ensureGroup missing Location header');
      return id;
    },

    async updateRealm(name: string, update: KeycloakRealmUpdate): Promise<boolean> {
      // Admin endpoints for realm management are not under
      // /admin/realms/{realm}/... — they are at /admin/realms/{realm}
      // directly. We still need a realm-context token for authn, so
      // route through adminFetch using the same realm.
      // GET first to surface the 404 case cleanly.
      const get = await adminFetch(name, '');
      if (get.status === 404) return false;
      if (!get.ok) {
        throw new Error(
          `keycloak-admin: realm ${name} lookup failed (${get.status}): ${await get.text()}`
        );
      }
      const current = (await get.json()) as Record<string, unknown>;
      const merged = { ...current, ...update, realm: name };
      const put = await adminFetch(name, '', {
        method: 'PUT',
        headers: { 'content-type': 'application/json' },
        body: JSON.stringify(merged),
      });
      if (!put.ok) {
        throw new Error(
          `keycloak-admin: realm ${name} update failed (${put.status}): ${await put.text()}`
        );
      }
      return true;
    },

    async ensureUser(spec: KeycloakUserSpec): Promise<string> {
      const realm = spec.realm ?? defaultRealm;

      const lookup = await adminFetch(
        realm,
        `/users?username=${encodeURIComponent(spec.username)}&exact=true`
      );
      if (!lookup.ok) {
        throw new Error(
          `keycloak-admin: ensureUser ${spec.username} lookup failed (${lookup.status}): ${await lookup.text()}`
        );
      }
      const existing = (await lookup.json()) as Array<{ id: string; username: string }>;
      let userId = existing.find((u) => u.username === spec.username)?.id;

      const body = {
        username: spec.username,
        email: spec.email,
        firstName: spec.firstName,
        lastName: spec.lastName,
        enabled: spec.enabled ?? true,
        // Mark email verified to avoid forcing first-login email-verify
        // flow; identity flow is governed by the realm config.
        emailVerified: spec.email ? true : undefined,
      };

      if (userId) {
        const upd = await adminFetch(realm, `/users/${userId}`, {
          method: 'PUT',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(body),
        });
        if (!upd.ok) {
          throw new Error(
            `keycloak-admin: ensureUser ${spec.username} update failed (${upd.status}): ${await upd.text()}`
          );
        }
      } else {
        const create = await adminFetch(realm, '/users', {
          method: 'POST',
          headers: { 'content-type': 'application/json' },
          body: JSON.stringify(body),
        });
        if (create.status === 409) {
          // Race: another caller created it. Re-lookup.
          const retry = await adminFetch(
            realm,
            `/users?username=${encodeURIComponent(spec.username)}&exact=true`
          );
          const arr = (await retry.json()) as Array<{ id: string; username: string }>;
          userId = arr.find((u) => u.username === spec.username)?.id;
        } else if (!create.ok) {
          throw new Error(
            `keycloak-admin: ensureUser ${spec.username} create failed (${create.status}): ${await create.text()}`
          );
        } else {
          const location = create.headers.get('location') ?? '';
          userId = location.split('/').pop() ?? '';
        }
        if (!userId) throw new Error('keycloak-admin: ensureUser missing user id');
      }

      // Sync group memberships if specified. We resolve each group
      // name to its id (creating groups in passing is out of scope —
      // those flow through the Group resource's own hook).
      if (spec.groups && spec.groups.length > 0) {
        for (const groupName of spec.groups) {
          const gLookup = await adminFetch(
            realm,
            `/groups?search=${encodeURIComponent(groupName)}&exact=true`
          );
          if (!gLookup.ok) continue;
          const groups = (await gLookup.json()) as Array<{ id: string; name: string }>;
          const found = groups.find((g) => g.name === groupName);
          if (!found) continue;
          await adminFetch(realm, `/users/${userId}/groups/${found.id}`, { method: 'PUT' });
        }
      }

      return userId;
    },

    async deleteUser(realm: string, username: string): Promise<void> {
      const r = realm || defaultRealm;
      const lookup = await adminFetch(
        r,
        `/users?username=${encodeURIComponent(username)}&exact=true`
      );
      if (!lookup.ok) return;
      const arr = (await lookup.json()) as Array<{ id: string; username: string }>;
      const found = arr.find((u) => u.username === username);
      if (!found) return;
      const del = await adminFetch(r, `/users/${found.id}`, { method: 'DELETE' });
      if (!del.ok && del.status !== 404) {
        throw new Error(
          `keycloak-admin: deleteUser ${username} failed (${del.status}): ${await del.text()}`
        );
      }
    },

    async deleteGroup(realm: string, name: string): Promise<void> {
      const r = realm || defaultRealm;
      const lookup = await adminFetch(r, `/groups?search=${encodeURIComponent(name)}&exact=true`);
      if (!lookup.ok) return;
      const groups = (await lookup.json()) as Array<{ id: string; name: string }>;
      const found = groups.find((g) => g.name === name);
      if (!found) return;
      const del = await adminFetch(r, `/groups/${found.id}`, { method: 'DELETE' });
      if (!del.ok && del.status !== 404) {
        throw new Error(
          `keycloak-admin: deleteGroup ${name} failed (${del.status}): ${await del.text()}`
        );
      }
    },
  };
}

function inferDefaultRealm(issuerUrl: string): string {
  const m = issuerUrl.match(/\/realms\/([^/]+)\/?$/);
  return m?.[1] ?? 'novanas';
}
