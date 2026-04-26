import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

const sample = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'IscsiTarget',
  metadata: { name: 'iscsi-a' },
  spec: {
    blockVolume: 'vol-a',
    portal: { hostInterface: 'eth0' },
  },
};

describe('iscsi-targets resource', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let userSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('iscsitargets', sample);
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    userSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  it('lists iscsi targets', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/iscsi-targets',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    expect((r.json() as { items: unknown[] }).items).toHaveLength(1);
  });

  it('admin CRUDs', async () => {
    const obj = { ...sample, metadata: { name: 'iscsi-b' } };
    const c = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/iscsi-targets',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: obj,
    });
    expect(c.statusCode).toBe(201);
    const d = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/iscsi-targets/iscsi-b',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(d.statusCode).toBe(204);
  });

  it('non-admin cannot write (403)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/iscsi-targets',
      headers: { cookie: cookieFor(h.built, userSid), 'content-type': 'application/json' },
      payload: { ...sample, metadata: { name: 'iscsi-c' } },
    });
    expect(r.statusCode).toBe(403);
  });

  it('404 on missing', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/iscsi-targets/missing',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
