import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

function sampleCert(name: string) {
  return {
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Certificate',
    metadata: { name },
    spec: { commonName: 'foo.example.com' },
  };
}

function sampleRepl(name: string) {
  return {
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ReplicationJob',
    metadata: { name },
    spec: { source: 'pool/ds', target: 'remote' },
  };
}

function sampleCloudBackup(name: string) {
  return {
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'CloudBackupJob',
    metadata: { name },
    spec: { source: 'pool/ds', target: 's3' },
  };
}

describe('data-protection action routes', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let viewerSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    await h.kube.seed('certificates', sampleCert('letsencrypt'));
    await h.kube.seed('replicationjobs', sampleRepl('rep1'));
    await h.kube.seed('cloudbackupjobs', sampleCloudBackup('cb1'));
    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    viewerSid = await h.authAs({ username: 'obs', roles: [AuthzRole.Viewer] });
  });
  afterAll(async () => h.built.app.close());

  it('POST /certificates/:name/renew (200)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/certificates/letsencrypt/renew',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('POST /replication-jobs/:name/run-now (200)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/replication-jobs/rep1/run-now',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('POST /replication-jobs/:name/cancel (200)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/replication-jobs/rep1/cancel',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('POST /cloud-backup-jobs/:name/run-now (200)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/cloud-backup-jobs/cb1/run-now',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('POST /cloud-backup-jobs/:name/cancel (200)', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/cloud-backup-jobs/cb1/cancel',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('404 on missing cert', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/certificates/ghost/renew',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(404);
  });
});
