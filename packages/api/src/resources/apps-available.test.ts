import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

const sample = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'App',
  metadata: { name: 'jellyfin' },
  spec: {
    displayName: 'Jellyfin',
    version: '10.8.0',
    chart: { name: 'jellyfin', version: '10.8.0' },
  },
};

describe('apps-available resource (read-only)', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let userSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('apps', sample);
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    userSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  it('lists available apps', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/apps-available',
      headers: { cookie: cookieFor(h.built, userSid) },
    });
    expect(r.statusCode).toBe(200);
    expect((r.json() as { items: unknown[] }).items).toHaveLength(1);
  });

  it('gets an available app by name', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/apps-available/jellyfin',
      headers: { cookie: cookieFor(h.built, userSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('POST returns 405', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/apps-available',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: sample,
    });
    expect(r.statusCode).toBe(405);
  });

  it('PATCH returns 405', async () => {
    const r = await h.built.app.inject({
      method: 'PATCH',
      url: '/api/v1/apps-available/jellyfin',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: { metadata: { labels: { k: 'v' } } },
    });
    expect(r.statusCode).toBe(405);
  });

  it('DELETE returns 405', async () => {
    const r = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/apps-available/jellyfin',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(405);
  });

  it('404 on missing', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/apps-available/nope',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
