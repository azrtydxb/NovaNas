import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

const sample = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'BackendAssignment',
  metadata: { name: 'ba-pool-a-node-1' },
  spec: {
    poolRef: 'pool-a',
    nodeName: 'node-1',
    backendType: 'raw',
    deviceFilter: { preferredClass: 'nvme' },
  },
};

describe('backend-assignments resource', () => {
  let h: TestAppHandle;
  let adminSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
  });
  afterAll(async () => h.built.app.close());

  it('admin can CRUD a BackendAssignment', async () => {
    const c = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/backend-assignments',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: sample,
    });
    expect(c.statusCode).toBe(201);

    const g = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/backend-assignments/ba-pool-a-node-1',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(g.statusCode).toBe(200);

    // Status patch (typical agent flow: write Phase, BdevName, Capacity)
    const p = await h.built.app.inject({
      method: 'PATCH',
      url: '/api/v1/backend-assignments/ba-pool-a-node-1',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: {
        status: {
          phase: 'Ready',
          device: '/dev/nvme0n1',
          bdevName: 'novanas_pool',
          capacity: 1024209543168,
        },
      },
    });
    expect(p.statusCode).toBe(200);
    const patched = p.json() as { status?: { phase?: string; bdevName?: string } };
    expect(patched.status?.phase).toBe('Ready');
    expect(patched.status?.bdevName).toBe('novanas_pool');

    const list = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/backend-assignments',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(list.statusCode).toBe(200);
    const body = list.json() as { items: unknown[] };
    expect(body.items).toHaveLength(1);

    const d = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/backend-assignments/ba-pool-a-node-1',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(d.statusCode).toBe(204);
  });

  it('rejects unknown backendType', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/backend-assignments',
      headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
      payload: {
        ...sample,
        metadata: { name: 'bad' },
        spec: { ...sample.spec, backendType: 'zfs' },
      },
    });
    expect(r.statusCode).toBe(400);
  });

  it('returns 404 for missing BackendAssignment', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/backend-assignments/nope',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
