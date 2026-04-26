import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { Env } from '../env.js';
import { createKeycloakAdmin } from './keycloak-admin.js';

const baseEnv: Env = {
  NODE_ENV: 'test',
  PORT: 8080,
  LOG_LEVEL: 'info',
  DATABASE_URL: 'postgres://x:y@localhost/z',
  REDIS_URL: 'memory://',
  KEYCLOAK_ISSUER_URL: 'https://kc.example.com/realms/novanas',
  KEYCLOAK_INTERNAL_ISSUER_URL: 'http://novanas-keycloak.novanas-system.svc/realms/novanas',
  KEYCLOAK_CLIENT_ID: 'novanas-api',
  KEYCLOAK_CLIENT_SECRET: 'secret',
  SESSION_COOKIE_NAME: 'novanas_session',
  SESSION_SECRET: 'a-very-long-test-secret',
  API_VERSION: '0.0.0',
  API_PUBLIC_URL: 'https://kc.example.com',
};

describe('keycloak-admin', () => {
  let fetchMock: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchMock = vi.fn();
    vi.stubGlobal('fetch', fetchMock);
  });

  afterEach(() => {
    vi.unstubAllGlobals();
  });

  function tokenResponse() {
    return new Response(JSON.stringify({ access_token: 'AT', expires_in: 60 }), {
      status: 200,
      headers: { 'content-type': 'application/json' },
    });
  }

  it('ensureGroup creates when not found', async () => {
    fetchMock
      // token
      .mockResolvedValueOnce(tokenResponse())
      // list (empty)
      .mockResolvedValueOnce(new Response(JSON.stringify([]), { status: 200 }))
      // create
      .mockResolvedValueOnce(
        new Response(null, {
          status: 201,
          headers: { Location: '/admin/realms/novanas/groups/abc-123' },
        })
      );

    const admin = createKeycloakAdmin(baseEnv);
    const id = await admin.ensureGroup({ name: 'engineering' });
    expect(id).toBe('abc-123');

    // Discovery should target the in-cluster URL (HTTP), not the public one.
    const tokenCall = fetchMock.mock.calls[0]?.[0] as string;
    expect(tokenCall).toContain('http://novanas-keycloak.novanas-system.svc');
  });

  it('ensureGroup returns existing id without creating', async () => {
    fetchMock
      .mockResolvedValueOnce(tokenResponse())
      .mockResolvedValueOnce(
        new Response(JSON.stringify([{ id: 'existing-id', name: 'engineering' }]), { status: 200 })
      );

    const admin = createKeycloakAdmin(baseEnv);
    const id = await admin.ensureGroup({ name: 'engineering' });
    expect(id).toBe('existing-id');
    // Two calls only: token + list. No POST.
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('deleteGroup is a no-op when group missing', async () => {
    fetchMock
      .mockResolvedValueOnce(tokenResponse())
      .mockResolvedValueOnce(new Response(JSON.stringify([]), { status: 200 }));

    const admin = createKeycloakAdmin(baseEnv);
    await admin.deleteGroup('', 'gone');
    // No DELETE call follows.
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('updateRealm merges updates into the existing realm representation', async () => {
    fetchMock
      .mockResolvedValueOnce(tokenResponse())
      // GET /admin/realms/novanas
      .mockResolvedValueOnce(
        new Response(
          JSON.stringify({ realm: 'novanas', displayName: 'Old', enabled: true, attributes: {} }),
          { status: 200 }
        )
      )
      // PUT /admin/realms/novanas
      .mockResolvedValueOnce(new Response(null, { status: 204 }));

    const admin = createKeycloakAdmin(baseEnv);
    const ok = await admin.updateRealm('novanas', { displayName: 'New', enabled: true });
    expect(ok).toBe(true);
    const putCall = fetchMock.mock.calls[2];
    const body = JSON.parse((putCall?.[1] as RequestInit).body as string);
    // Existing fields are preserved; the update is merged on top.
    expect(body).toMatchObject({ realm: 'novanas', displayName: 'New', attributes: {} });
  });

  it('updateRealm returns false on 404', async () => {
    fetchMock
      .mockResolvedValueOnce(tokenResponse())
      .mockResolvedValueOnce(new Response('not found', { status: 404 }));
    const admin = createKeycloakAdmin(baseEnv);
    const ok = await admin.updateRealm('nope', { displayName: 'x' });
    expect(ok).toBe(false);
    // No PUT issued.
    expect(fetchMock).toHaveBeenCalledTimes(2);
  });

  it('retries once on 401 by refreshing the cached token', async () => {
    fetchMock
      .mockResolvedValueOnce(tokenResponse())
      // first list returns 401 (token expired in cache window)
      .mockResolvedValueOnce(new Response('expired', { status: 401 }))
      // refresh
      .mockResolvedValueOnce(tokenResponse())
      // retried list succeeds
      .mockResolvedValueOnce(new Response(JSON.stringify([]), { status: 200 }))
      // create
      .mockResolvedValueOnce(
        new Response(null, {
          status: 201,
          headers: { Location: '/admin/realms/novanas/groups/new-id' },
        })
      );

    const admin = createKeycloakAdmin(baseEnv);
    const id = await admin.ensureGroup({ name: 'team' });
    expect(id).toBe('new-id');
  });
});
