import type { CustomObjectsApi } from '@kubernetes/client-node';
import pino from 'pino';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { type BuiltApp, buildApp } from '../app.js';
import { AuthzRole } from '../auth/authz.js';
import {
  FakeCustomObjectsApi,
  cookieFor,
  fakeKeycloak,
  fakeRedis,
  testEnv,
} from '../resources/_test-helpers.js';

describe('system info/network/alerts/events routes', () => {
  let built: BuiltApp;
  let sid: string;

  beforeAll(async () => {
    built = await buildApp({
      env: testEnv,
      logger: pino({ level: 'silent' }),
      redis: fakeRedis(),
      keycloak: fakeKeycloak(),
      kubeCustom: new FakeCustomObjectsApi() as unknown as CustomObjectsApi,
      disableSwagger: true,
      disablePubSub: true,
    });
    sid = await built.sessions.create({
      userId: 'u',
      username: 'admin',
      createdAt: Date.now(),
      expiresAt: Date.now() + 3600_000,
      idToken: 't',
      accessToken: 't',
      claims: {
        sub: 'u',
        preferred_username: 'admin',
        realm_access: { roles: [AuthzRole.Admin] },
      },
    });
  });

  afterAll(async () => built.app.close());

  it('GET /system/info returns host metrics', async () => {
    const r = await built.app.inject({
      method: 'GET',
      url: '/api/v1/system/info',
      headers: { cookie: cookieFor(built, sid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as {
      hostname: string;
      cpuCount: number;
      cpuModel: string;
      totalMemoryBytes: number;
      uptimeSeconds: number;
      loadAvg: number[];
    };
    expect(typeof body.hostname).toBe('string');
    expect(body.cpuCount).toBeGreaterThan(0);
    expect(typeof body.cpuModel).toBe('string');
    expect(body.totalMemoryBytes).toBeGreaterThan(0);
    expect(body.uptimeSeconds).toBeGreaterThanOrEqual(0);
    expect(Array.isArray(body.loadAvg)).toBe(true);
  });

  it('GET /system/network returns interface list', async () => {
    const r = await built.app.inject({
      method: 'GET',
      url: '/api/v1/system/network',
      headers: { cookie: cookieFor(built, sid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { interfaces: Array<{ name: string; addresses: unknown[] }> };
    expect(Array.isArray(body.interfaces)).toBe(true);
  });

  it('GET /system/alerts returns empty list by default', async () => {
    const r = await built.app.inject({
      method: 'GET',
      url: '/api/v1/system/alerts',
      headers: { cookie: cookieFor(built, sid) },
    });
    expect(r.statusCode).toBe(200);
    expect(r.json()).toEqual({ items: [] });
  });

  it('GET /system/events returns empty list by default', async () => {
    const r = await built.app.inject({
      method: 'GET',
      url: '/api/v1/system/events',
      headers: { cookie: cookieFor(built, sid) },
    });
    expect(r.statusCode).toBe(200);
    expect(r.json()).toEqual({ items: [] });
  });
});

describe('CRD routes fallback when kubeCustom is missing', () => {
  let built: BuiltApp;
  let sid: string;

  beforeAll(async () => {
    built = await buildApp({
      env: testEnv,
      logger: pino({ level: 'silent' }),
      redis: fakeRedis(),
      keycloak: fakeKeycloak(),
      disableSwagger: true,
      disablePubSub: true,
      // kubeCustom omitted on purpose
    });
    sid = await built.sessions.create({
      userId: 'u',
      username: 'admin',
      createdAt: Date.now(),
      expiresAt: Date.now() + 3600_000,
      idToken: 't',
      accessToken: 't',
      claims: {
        sub: 'u',
        preferred_username: 'admin',
        realm_access: { roles: [AuthzRole.Admin] },
      },
    });
  });

  afterAll(async () => built.app.close());

  it('GET /api/v1/pools returns 503 kube_unavailable', async () => {
    const r = await built.app.inject({
      method: 'GET',
      url: '/api/v1/pools',
      headers: { cookie: cookieFor(built, sid) },
    });
    expect(r.statusCode).toBe(503);
    const body = r.json() as { error: string; message: string };
    expect(body.error).toBe('kube_unavailable');
    expect(typeof body.message).toBe('string');
    // Ensure the old stub envelope is gone.
    expect(body).not.toHaveProperty('wave');
  });
});
