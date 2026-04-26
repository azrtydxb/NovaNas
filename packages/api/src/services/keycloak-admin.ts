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

export interface KeycloakAdmin {
  /** Idempotent group ensure. Returns the Keycloak group id. */
  ensureGroup(spec: KeycloakGroupSpec): Promise<string>;
  /** Best-effort group delete. Missing group is not an error. */
  deleteGroup(realm: string, name: string): Promise<void>;
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

  async function adminFetch(realm: string, path: string, init: RequestInit = {}): Promise<Response> {
    const url = `${baseUrl}/admin/realms/${realm}${path}`;
    let token = await getToken(realm);
    let res = await fetch(url, {
      ...init,
      headers: { ...(init.headers ?? {}), authorization: `Bearer ${token}` },
    });
    if (res.status === 401) {
      // Token rotated under us — invalidate and retry once.
      tokens.delete(realm);
      token = await getToken(realm);
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
      const create = await adminFetch(realm, `/groups`, {
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
