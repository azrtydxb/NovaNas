import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

const sample = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'BucketUser',
  metadata: { name: 'bu-a' },
  spec: {
    credentials: {},
  },
};

describe('bucket-users resource', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let viewerSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('bucketusers', sample);
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    viewerSid = await h.authAs({ username: 'vic', roles: [AuthzRole.Viewer] });
  });
  afterAll(async () => h.built.app.close());

  it('lists bucket users', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/bucket-users',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    expect((r.json() as { items: unknown[] }).items).toHaveLength(1);
  });

  it('CRUDs', async () => {
    const obj = { ...sample, metadata: { name: 'bu-b' } };
    const c = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/bucket-users',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: obj,
    });
    expect(c.statusCode).toBe(201);
    const d = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/bucket-users/bu-b',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(d.statusCode).toBe(204);
  });

  it('viewer cannot write (403)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/bucket-users',
      headers: { cookie: cookieFor(h.built, viewerSid), 'content-type': 'application/json' },
      payload: { ...sample, metadata: { name: 'bu-c' } },
    });
    expect(r.statusCode).toBe(403);
  });

  it('404 on missing', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/bucket-users/missing',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
