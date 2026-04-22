import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

const sample = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'Disk',
  metadata: { name: 'disk-1' },
  spec: { role: 'data' },
};

describe('disks resource', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let userSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    h.kube.seed('disks', sample);
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    userSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  it('lists', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/disks',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('gets', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/disks/disk-1',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('admin creates/patches/deletes', async () => {
    const obj = { ...sample, metadata: { name: 'disk-2' } };
    const c = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/disks',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: obj,
    });
    expect(c.statusCode).toBe(201);
    const p = await h.built.app.inject({
      method: 'PATCH',
      url: '/api/v1/disks/disk-2',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: { metadata: { labels: { tag: 't' } } },
    });
    expect(p.statusCode).toBe(200);
    const d = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/disks/disk-2',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(d.statusCode).toBe(204);
  });

  it('user cannot write (403)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/disks',
      headers: { cookie: cookieFor(h.built, userSid), 'content-type': 'application/json' },
      payload: { ...sample, metadata: { name: 'disk-x' } },
    });
    expect(r.statusCode).toBe(403);
  });

  it('404 missing', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/disks/missing',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
