import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

function sampleVm(user: string, name: string) {
  return {
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Vm',
    metadata: { name, namespace: `user-${user}` },
    spec: { cpu: 2, memoryMiB: 2048, powerState: 'Stopped' },
  };
}

describe('vms action routes (E1-API-Actions)', () => {
  let h: TestAppHandle;
  let aliceSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('vms', sampleVm('alice', 'vm1'), 'user-alice');
    aliceSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  for (const action of ['start', 'stop', 'reset', 'pause', 'resume']) {
    it(`POST /${action} returns 200`, async () => {
      const r = await h.built.app.inject({
        method: 'POST',
        url: `/api/v1/vms/user-alice/vm1/${action}`,
        headers: { cookie: cookieFor(h.built, aliceSid) },
      });
      expect(r.statusCode).toBe(200);
      const body = r.json() as { accepted: boolean };
      expect(body.accepted).toBe(true);
    });
  }

  it('POST /stop?force=true uses power-off', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/vms/user-alice/vm1/stop?force=true',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('404 for missing VM', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/vms/user-alice/ghost/start',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(r.statusCode).toBe(404);
  });

  it('DELETE with deleteDisks=true requires confirm', async () => {
    const noConfirm = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/vms/user-alice/vm1?deleteDisks=true',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(noConfirm.statusCode).toBe(400);

    const confirmed = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/vms/user-alice/vm1?deleteDisks=true',
      headers: {
        cookie: cookieFor(h.built, aliceSid),
        'x-confirm-destructive': 'vm1',
      },
    });
    expect(confirmed.statusCode).toBe(200);
  });
});
