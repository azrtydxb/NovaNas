import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

const sample = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'IsoLibrary',
  metadata: { name: 'iso-a' },
  spec: { dataset: 'pool/isos' },
};

describe('iso-libraries resource', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let userSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    h.kube.seed('isolibraries', sample);
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    userSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  it('lists iso libraries', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/iso-libraries',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    expect((r.json() as { items: unknown[] }).items).toHaveLength(1);
  });

  it('admin CRUDs', async () => {
    const obj = { ...sample, metadata: { name: 'iso-b' } };
    const c = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/iso-libraries',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: obj,
    });
    expect(c.statusCode).toBe(201);
    const d = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/iso-libraries/iso-b',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(d.statusCode).toBe(204);
  });

  it('non-admin cannot write (403)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/iso-libraries',
      headers: { cookie: cookieFor(h.built, userSid), 'content-type': 'application/json' },
      payload: { ...sample, metadata: { name: 'iso-c' } },
    });
    expect(r.statusCode).toBe(403);
  });

  it('404 on missing', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/iso-libraries/missing',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
