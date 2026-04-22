import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

describe('scrub-schedules resource', () => {
  let h: TestAppHandle;
  let adminSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
  });
  afterAll(async () => h.built.app.close());

  it('lists ScrubSchedules (empty) for admin', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/scrub-schedules',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { items: unknown[] };
    expect(Array.isArray(body.items)).toBe(true);
  });

  it('returns 403 for share-only on list', async () => {
    const shareSid = await h.authAs({ username: 'guest', roles: ['novanas:share-only'] });
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/scrub-schedules',
      headers: { cookie: cookieFor(h.built, shareSid) },
    });
    expect(r.statusCode).toBe(403);
  });

  it('returns 404 for missing ScrubSchedule', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/scrub-schedules/missing',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });

  it('requires authentication', async () => {
    const r = await h.built.app.inject({ method: 'GET', url: '/api/v1/scrub-schedules' });
    expect(r.statusCode).toBe(401);
  });
});
