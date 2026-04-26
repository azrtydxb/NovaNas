import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

const sample = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'StoragePool',
  metadata: { name: 'pool-a' },
  spec: { tier: '1' },
};

describe('pools resource', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let userSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('storagepools', sample);
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    userSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  it('lists pools for admin', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/pools',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { items: unknown[] };
    expect(body.items).toHaveLength(1);
  });

  it('gets a pool by name', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/pools/pool-a',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('creates a pool', async () => {
    const obj = {
      apiVersion: 'novanas.io/v1alpha1',
      kind: 'StoragePool',
      metadata: { name: 'pool-b' },
      spec: { tier: '4' },
    };
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/pools',
      headers: {
        cookie: cookieFor(h.built, adminSid),
        'content-type': 'application/json',
      },
      payload: obj,
    });
    expect(r.statusCode).toBe(201);
  });

  it('patches a pool', async () => {
    const r = await h.built.app.inject({
      method: 'PATCH',
      url: '/api/v1/pools/pool-a',
      headers: {
        cookie: cookieFor(h.built, adminSid),
        'content-type': 'application/json',
      },
      payload: { metadata: { name: 'pool-a', labels: { env: 'prod' } } },
    });
    expect(r.statusCode).toBe(200);
  });

  it('deletes a pool', async () => {
    const r = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/pools/pool-b',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(204);
  });

  it('returns 403 for non-admin writes', async () => {
    const r = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/pools/pool-a',
      headers: { cookie: cookieFor(h.built, userSid) },
    });
    expect(r.statusCode).toBe(403);
  });

  it('returns 404 for missing resources', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/pools/missing',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
