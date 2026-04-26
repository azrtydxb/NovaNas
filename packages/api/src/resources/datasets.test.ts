import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

const sample = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'Dataset',
  metadata: { name: 'ds-a' },
  spec: { pool: 'pool-a', size: '10Gi', filesystem: 'xfs' },
};

describe('datasets resource', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let viewerSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('datasets', sample);
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    viewerSid = await h.authAs({ username: 'vic', roles: [AuthzRole.Viewer] });
  });
  afterAll(async () => h.built.app.close());

  it('lists datasets', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/datasets',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    expect((r.json() as { items: unknown[] }).items).toHaveLength(1);
  });

  it('gets, creates, patches, deletes', async () => {
    const obj = { ...sample, metadata: { name: 'ds-b' } };
    const create = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/datasets',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: obj,
    });
    expect(create.statusCode).toBe(201);

    const get = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/datasets/ds-b',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(get.statusCode).toBe(200);

    const patch = await h.built.app.inject({
      method: 'PATCH',
      url: '/api/v1/datasets/ds-b',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: { metadata: { labels: { env: 'dev' } } },
    });
    expect(patch.statusCode).toBe(200);

    const del = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/datasets/ds-b',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(del.statusCode).toBe(204);
  });

  it('viewer cannot delete (403)', async () => {
    const r = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/datasets/ds-a',
      headers: { cookie: cookieFor(h.built, viewerSid) },
    });
    expect(r.statusCode).toBe(403);
  });

  it('returns 404 for missing', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/datasets/nope',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
