import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from '../resources/_test-helpers.js';

describe('auth device-code + token endpoints (CLI flow)', () => {
  let h: TestAppHandle;
  let sid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    sid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  it('POST /auth/token rejects missing device_code (400)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/auth/token',
      headers: { 'content-type': 'application/json' },
      payload: {},
    });
    expect(r.statusCode).toBe(400);
  });

  it('POST /auth/device-code reaches upstream (502 in tests — no real keycloak)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/auth/device-code',
    });
    // either 502 (unreachable) or 200 if fetch mocked — must not be 404
    expect([200, 502]).toContain(r.statusCode);
  });

  it('GET /auth/me returns synthetic admin (auth disabled)', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/auth/me',
      headers: { cookie: cookieFor(h.built, sid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { username: string };
    expect(body.username).toBe('admin');
  });
});
