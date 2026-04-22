import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

describe('gpu-devices resource', () => {
  let h: TestAppHandle;
  let adminSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
  });
  afterAll(async () => h.built.app.close());

  it('lists GpuDevices (empty) for admin', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/gpu-devices',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { items: unknown[] };
    expect(Array.isArray(body.items)).toBe(true);
  });

  it('rejects POST with 405', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/gpu-devices',
      headers: {
        cookie: cookieFor(h.built, adminSid),
        'content-type': 'application/json',
      },
      payload: {},
    });
    expect(r.statusCode).toBe(405);
  });

  it('rejects PATCH with 405', async () => {
    const r = await h.built.app.inject({
      method: 'PATCH',
      url: '/api/v1/gpu-devices/sample-1',
      headers: {
        cookie: cookieFor(h.built, adminSid),
        'content-type': 'application/json',
      },
      payload: {},
    });
    expect(r.statusCode).toBe(405);
  });

  it('rejects DELETE with 405', async () => {
    const r = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/gpu-devices/sample-1',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(405);
  });

  it('requires authentication', async () => {
    const r = await h.built.app.inject({ method: 'GET', url: '/api/v1/gpu-devices' });
    expect(r.statusCode).toBe(401);
  });
});
