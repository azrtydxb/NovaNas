import type { CustomObjectsApi } from '@kubernetes/client-node';
import pino from 'pino';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
import { type BuiltApp, buildApp } from '../app.js';
import { AuthzRole } from '../auth/authz.js';
import {
  FakeCustomObjectsApi,
  cookieFor,
  fakeKeycloak,
  fakeRedis,
  testEnv,
} from '../resources/_test-helpers.js';
import { createPromClient } from '../services/prom.js';
import { parseRange, stepFor } from './metrics.js';

describe('parseRange + stepFor', () => {
  it('parses h/d/m', () => {
    expect(parseRange('1h', 0)).toBe(3600);
    expect(parseRange('7d', 0)).toBe(7 * 86400);
    expect(parseRange('30m', 0)).toBe(30 * 60);
    expect(parseRange(undefined, 42)).toBe(42);
  });
  it('picks a sensible step', () => {
    expect(stepFor(3600)).toBeLessThanOrEqual(60);
    expect(stepFor(7 * 86400)).toBeGreaterThanOrEqual(300);
  });
});

describe('metrics routes', () => {
  let built: BuiltApp;
  let sid: string;
  let fetchImpl: ReturnType<typeof vi.fn>;

  beforeAll(async () => {
    fetchImpl = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({
        status: 'success',
        data: {
          resultType: 'matrix',
          result: [{ metric: { pool: 'a' }, values: [[100, '1']] }],
        },
      }),
    });
    const env = { ...testEnv, PROMETHEUS_URL: 'http://prom.test' };
    const prom = createPromClient(env, { fetchImpl: fetchImpl as unknown as typeof fetch });
    built = await buildApp({
      env,
      logger: pino({ level: 'silent' }),
      redis: fakeRedis(),
      keycloak: fakeKeycloak(),
      kubeCustom: new FakeCustomObjectsApi() as unknown as CustomObjectsApi,
      disableSwagger: true,
      disablePubSub: true,
      prom,
    });
    sid = await built.sessions.create({
      userId: 'u',
      username: 'u',
      createdAt: Date.now(),
      expiresAt: Date.now() + 3600_000,
      idToken: 't',
      accessToken: 't',
      claims: { sub: 'u', preferred_username: 'u', realm_access: { roles: [AuthzRole.Admin] } },
    });
  });

  afterAll(async () => {
    await built.app.close();
  });

  it('GET /api/v1/metrics/pool/:name/throughput returns structured series', async () => {
    const r = await built.app.inject({
      method: 'GET',
      url: '/api/v1/metrics/pool/a/throughput?range=1h',
      headers: { cookie: cookieFor(built, sid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as {
      scope: string;
      query: string;
      range: { start: string; end: string; stepSeconds: number };
      series: Array<{ labels: Record<string, string>; points: Array<{ t: number; v: number }> }>;
    };
    expect(body.scope).toBe('pool:a:throughput');
    expect(body.query).toContain('novanas_pool_bytes_total');
    expect(body.series).toHaveLength(1);
    expect(body.series[0]!.points[0]).toEqual({ t: 100, v: 1 });
    expect(fetchImpl).toHaveBeenCalled();
    const url = fetchImpl.mock.calls[0]![0] as string;
    expect(url).toContain('/api/v1/query_range');
  });

  it('GET /api/v1/metrics ad-hoc with explicit query', async () => {
    const r = await built.app.inject({
      method: 'GET',
      url: '/api/v1/metrics?scope=custom&query=up&range=30m',
      headers: { cookie: cookieFor(built, sid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { scope: string; query: string };
    expect(body.scope).toBe('custom');
    expect(body.query).toBe('up');
  });

  it('returns 503 when prom client missing', async () => {
    const built2 = await buildApp({
      env: testEnv,
      logger: pino({ level: 'silent' }),
      redis: fakeRedis(),
      keycloak: fakeKeycloak(),
      disableSwagger: true,
      disablePubSub: true,
    });
    const sid2 = await built2.sessions.create({
      userId: 'u',
      username: 'u',
      createdAt: Date.now(),
      expiresAt: Date.now() + 3600_000,
      idToken: 't',
      accessToken: 't',
      claims: { sub: 'u', preferred_username: 'u', realm_access: { roles: [AuthzRole.Admin] } },
    });
    const r = await built2.app.inject({
      method: 'GET',
      url: '/api/v1/metrics?query=up',
      headers: { cookie: cookieFor(built2, sid2) },
    });
    expect(r.statusCode).toBe(503);
    await built2.app.close();
  });
});
