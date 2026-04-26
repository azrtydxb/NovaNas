import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

function sampleGpu(name: string) {
  return {
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'GpuDevice',
    metadata: { name },
    spec: { pciId: '0000:01:00.0' },
  };
}

describe('gpu-devices action routes', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let userSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('gpudevices', sampleGpu('gpu0'));
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    userSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  it('POST /assign patches spec (admin)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/gpu-devices/gpu0/assign',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: { vmNamespace: 'user-alice', vmName: 'vm1' },
    });
    expect(r.statusCode).toBe(200);
  });

  it('POST /unassign returns 200 (admin)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/gpu-devices/gpu0/unassign',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('non-admin gets 403 (GpuDevice is admin-only-write)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/gpu-devices/gpu0/unassign',
      headers: { cookie: cookieFor(h.built, userSid) },
    });
    expect(r.statusCode).toBe(403);
  });

  it('400 on missing body', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/gpu-devices/gpu0/assign',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: { vmNamespace: 'user-alice' },
    });
    expect(r.statusCode).toBe(400);
  });

  it('404 on missing device', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/gpu-devices/ghost/unassign',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
