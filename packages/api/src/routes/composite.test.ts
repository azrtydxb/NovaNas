import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from '../resources/_test-helpers.js';

const dataset = {
  apiVersion: 'novanas.io/v1alpha1',
  kind: 'Dataset',
  metadata: { name: 'ds-comp' },
  spec: { pool: 'pool-a', size: '10Gi', filesystem: 'xfs' },
};

function shareBody(name: string, ds = 'ds-comp') {
  return {
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Share',
    metadata: { name },
    spec: {
      dataset: ds,
      path: `/${name}`,
      protocols: { smb: { server: 'smb-a' } },
    },
  };
}

describe('composite routes', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let userSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    userSid = await h.authAs({ username: 'pascal', roles: [AuthzRole.User] });
  });
  afterAll(async () => h.built.app.close());

  // --------------------------------------------------------------------------
  describe('POST /api/v1/composite/dataset-with-share', () => {
    it('creates dataset + shares atomically (happy path)', async () => {
      const r = await h.built.app.inject({
        method: 'POST',
        url: '/api/v1/composite/dataset-with-share',
        headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
        payload: { dataset, shares: [shareBody('sh-a'), shareBody('sh-b')] },
      });
      expect(r.statusCode).toBe(201);
      const body = r.json() as {
        dataset: { metadata: { name: string } };
        shares: Array<{ metadata: { name: string } }>;
      };
      expect(body.dataset.metadata.name).toBe('ds-comp');
      expect(body.shares).toHaveLength(2);
    });

    it('rolls back dataset when a share fails', async () => {
      // Seed a conflicting share name so the second step fails with 409.
      await h.kube.seed('shares', {
        apiVersion: 'novanas.io/v1alpha1',
        kind: 'Share',
        metadata: { name: 'sh-conflict' },
        spec: { dataset: 'x', path: '/x', protocols: {} },
      });

      const payload = {
        dataset: { ...dataset, metadata: { name: 'ds-rollback' } },
        shares: [shareBody('sh-conflict', 'ds-rollback')],
      };

      const r = await h.built.app.inject({
        method: 'POST',
        url: '/api/v1/composite/dataset-with-share',
        headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
        payload,
      });
      expect(r.statusCode).toBe(409);

      // Dataset must have been rolled back.
      const check = await h.built.app.inject({
        method: 'GET',
        url: '/api/v1/datasets/ds-rollback',
        headers: { cookie: cookieFor(h.built, adminSid) },
      });
      expect(check.statusCode).toBe(404);
    });

    it('returns 400 for missing shares', async () => {
      const r = await h.built.app.inject({
        method: 'POST',
        url: '/api/v1/composite/dataset-with-share',
        headers: { cookie: cookieFor(h.built, adminSid), 'content-type': 'application/json' },
        payload: { dataset, shares: [] },
      });
      expect(r.statusCode).toBe(400);
    });
  });

  // --------------------------------------------------------------------------
  describe('POST /api/v1/composite/install-app', () => {
    it('creates AppInstance with auto dataset', async () => {
      const r = await h.built.app.inject({
        method: 'POST',
        url: '/api/v1/composite/install-app',
        headers: { cookie: cookieFor(h.built, userSid), 'content-type': 'application/json' },
        payload: {
          app: 'plex',
          version: '1.40.3.8555',
          namespace: 'user-pascal',
          values: { foo: 'bar' },
          autoDataset: { name: 'plex-config', size: '5Gi', pool: 'main' },
        },
      });
      expect(r.statusCode).toBe(201);
      const body = r.json() as {
        appInstance: { metadata: { name: string } };
        dataset?: { metadata: { name: string } };
      };
      expect(body.appInstance.metadata.name).toBe('plex');
      expect(body.dataset?.metadata.name).toBe('plex-config');
    });

    it('rolls back dataset when AppInstance creation fails', async () => {
      // Pre-seed a conflicting AppInstance.
      await h.kube.seed(
        'appinstances',
        {
          apiVersion: 'novanas.io/v1alpha1',
          kind: 'AppInstance',
          metadata: { name: 'radarr', namespace: 'user-pascal' },
          spec: { app: 'radarr', version: '1.0.0' },
        },
        'user-pascal'
      );

      const r = await h.built.app.inject({
        method: 'POST',
        url: '/api/v1/composite/install-app',
        headers: { cookie: cookieFor(h.built, userSid), 'content-type': 'application/json' },
        payload: {
          app: 'radarr',
          version: '1.0.1',
          namespace: 'user-pascal',
          autoDataset: { name: 'radarr-config', size: '1Gi', pool: 'main' },
        },
      });
      expect(r.statusCode).toBe(409);

      const check = await h.built.app.inject({
        method: 'GET',
        url: '/api/v1/datasets/radarr-config',
        headers: { cookie: cookieFor(h.built, adminSid) },
      });
      expect(check.statusCode).toBe(404);
    });
  });

  // --------------------------------------------------------------------------
  describe('POST /api/v1/composite/create-vm', () => {
    const vm = {
      apiVersion: 'novanas.io/v1alpha1',
      kind: 'Vm',
      metadata: { name: 'vm-a', namespace: 'user-pascal' },
      spec: {
        os: { type: 'linux' },
        resources: { cpu: 2, memoryMiB: 2048 },
      },
    };

    it('creates VM with disks (happy path)', async () => {
      const r = await h.built.app.inject({
        method: 'POST',
        url: '/api/v1/composite/create-vm',
        headers: { cookie: cookieFor(h.built, userSid), 'content-type': 'application/json' },
        payload: {
          vm,
          disks: [
            { name: 'system', size: '80Gi', pool: 'main' },
            { name: 'data', size: '500Gi', pool: 'cold' },
          ],
        },
      });
      expect(r.statusCode).toBe(201);
      const body = r.json() as {
        vm: { metadata: { name: string } };
        disks: Array<{ metadata: { name: string } }>;
      };
      expect(body.vm.metadata.name).toBe('vm-a');
      expect(body.disks.map((d) => d.metadata.name)).toEqual(['vm-a-system', 'vm-a-data']);
    });

    it('rolls back disks when VM create fails', async () => {
      // Pre-seed a conflicting VM to force VM create to fail.
      await h.kube.seed(
        'vms',
        {
          apiVersion: 'novanas.io/v1alpha1',
          kind: 'Vm',
          metadata: { name: 'vm-conflict', namespace: 'user-pascal' },
          spec: { os: { type: 'linux' }, resources: { cpu: 1, memoryMiB: 1024 } },
        },
        'user-pascal'
      );

      const r = await h.built.app.inject({
        method: 'POST',
        url: '/api/v1/composite/create-vm',
        headers: { cookie: cookieFor(h.built, userSid), 'content-type': 'application/json' },
        payload: {
          vm: { ...vm, metadata: { name: 'vm-conflict', namespace: 'user-pascal' } },
          disks: [{ name: 'sys', size: '10Gi', pool: 'main' }],
        },
      });
      expect(r.statusCode).toBe(409);

      // Disk must have been rolled back.
      const check = await h.built.app.inject({
        method: 'GET',
        url: '/api/v1/disks/vm-conflict-sys',
        headers: { cookie: cookieFor(h.built, adminSid) },
      });
      expect(check.statusCode).toBe(404);
    });
  });
});
