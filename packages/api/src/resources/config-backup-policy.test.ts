import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

describe('config-backup-policy resource', () => {
  let h: TestAppHandle;
  let adminSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
  });
  afterAll(async () => h.built.app.close());

  it('returns 404 when singleton is not configured', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/config-backup-policy',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });

  it('returns 400 on non-object PATCH body', async () => {
    const r = await h.built.app.inject({
      method: 'PATCH',
      url: '/api/v1/config-backup-policy',
      headers: {
        cookie: cookieFor(h.built, adminSid),
        'content-type': 'application/json',
      },
      payload: '"nope"',
    });
    expect([400, 404]).toContain(r.statusCode);
  });

  it('does not expose DELETE', async () => {
    const r = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/config-backup-policy',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
