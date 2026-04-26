import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

function sampleFor(user: string, name: string) {
  return {
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Vm',
    metadata: { name, namespace: `user-${user}` },
    spec: {
      os: { type: 'linux' },
      resources: { cpu: 2, memoryMiB: 2048 },
      disks: [],
    },
  };
}

describe('vms resource (namespaced)', () => {
  let h: TestAppHandle;
  let aliceSid: string;
  let shareOnlySid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('vms', sampleFor('alice', 'vm-a'), 'user-alice');
    aliceSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
    shareOnlySid = await h.authAs({ username: 'bob', roles: [AuthzRole.ShareOnly] });
  });
  afterAll(async () => h.built.app.close());

  it('user lists own vms', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/vms',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(r.statusCode).toBe(200);
    expect((r.json() as { items: unknown[] }).items).toHaveLength(1);
  });

  it('user CRUDs own vm', async () => {
    const obj = sampleFor('alice', 'vm-b');
    const c = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/vms',
      headers: { cookie: cookieFor(h.built, aliceSid), 'content-type': 'application/json' },
      payload: obj,
    });
    expect(c.statusCode).toBe(201);
    const g = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/vms/vm-b',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(g.statusCode).toBe(200);
    const p = await h.built.app.inject({
      method: 'PATCH',
      url: '/api/v1/vms/vm-b',
      headers: { cookie: cookieFor(h.built, aliceSid), 'content-type': 'application/json' },
      payload: { metadata: { labels: { pinned: 'true' } } },
    });
    expect(p.statusCode).toBe(200);
    const d = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/vms/vm-b',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(d.statusCode).toBe(204);
  });

  it('share-only role cannot read (403)', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/vms',
      headers: { cookie: cookieFor(h.built, shareOnlySid) },
    });
    expect(r.statusCode).toBe(403);
  });

  it('404 missing', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/vms/nope',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
