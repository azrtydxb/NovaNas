import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

describe('user action routes (reset-password, enroll-2fa)', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let aliceSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    aliceSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  it('admin can reset-password', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/users/alice/reset-password',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { accepted: boolean };
    expect(body.accepted).toBe(true);
  });

  it('user can enroll themselves in 2FA', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/users/alice/enroll-2fa',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { secret: string; otpauthUrl: string };
    expect(body.secret).toMatch(/^[A-Z2-7]+$/);
    expect(body.otpauthUrl).toContain('otpauth://totp/');
  });
});
