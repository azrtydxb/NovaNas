import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

const sample = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'User',
  metadata: { name: 'alice' },
  spec: { username: 'alice' },
};

describe('users resource', () => {
  let h: TestAppHandle;
  let adminSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('users', sample);
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
  });
  afterAll(async () => h.built.app.close());

  it('admin lists', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/users',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('admin CRUDs', async () => {
    const obj = { ...sample, metadata: { name: 'charlie' }, spec: { username: 'charlie' } };
    const c = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/users',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: obj,
    });
    expect(c.statusCode).toBe(201);
    const g = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/users/charlie',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(g.statusCode).toBe(200);
    const p = await h.built.app.inject({
      method: 'PATCH',
      url: '/api/v1/users/charlie',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: { metadata: { labels: { k: 'v' } } },
    });
    expect(p.statusCode).toBe(200);
    const d = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/users/charlie',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(d.statusCode).toBe(204);
  });

  it('404 missing', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/users/zzzz',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
