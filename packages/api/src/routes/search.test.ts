import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from '../resources/_test-helpers.js';

describe('search route', () => {
  let h: TestAppHandle;
  let adminSid: string;
  let viewerSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    // Seed a handful of resources so the cross-resource fan-out has something
    // to return.
    await h.kube.seed('storagepools', {
      apiVersion: 'novanas.io/v1alpha1',
      kind: 'StoragePool',
      metadata: { name: 'pool-foo' },
      spec: { tier: 'hot' },
    });
    await h.kube.seed('storagepools', {
      apiVersion: 'novanas.io/v1alpha1',
      kind: 'StoragePool',
      metadata: { name: 'pool-bar', labels: { match: 'foo' } },
      spec: { tier: 'cold' },
    });
    await h.kube.seed('datasets', {
      apiVersion: 'novanas.io/v1alpha1',
      kind: 'Dataset',
      metadata: { name: 'ds-foo-a' },
      spec: { pool: 'pool-foo', size: '10Gi', filesystem: 'xfs' },
    });
    await h.kube.seed('datasets', {
      apiVersion: 'novanas.io/v1alpha1',
      kind: 'Dataset',
      metadata: { name: 'unrelated' },
      spec: { pool: 'pool-foo', size: '10Gi', filesystem: 'xfs' },
    });

    adminSid = await h.authAs({ username: 'admin', roles: [AuthzRole.Admin] });
    viewerSid = await h.authAs({ username: 'vic', roles: [AuthzRole.Viewer] });
  });

  afterAll(async () => h.built.app.close());

  it('returns grouped results for substring name matches', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/search?q=foo',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { results: Record<string, unknown[]> };
    expect(body.results).toBeDefined();
    expect(body.results.pools).toBeDefined();
    // pool-foo matches on name, pool-bar matches on label value 'foo'
    const poolNames = (body.results.pools as Array<{ metadata: { name: string } }>).map(
      (p) => p.metadata.name
    );
    expect(poolNames).toContain('pool-foo');
    expect(poolNames).toContain('pool-bar');

    const dsNames = (body.results.datasets as Array<{ metadata: { name: string } }>).map(
      (d) => d.metadata.name
    );
    expect(dsNames).toContain('ds-foo-a');
    expect(dsNames).not.toContain('unrelated');
  });

  it('is case-insensitive', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/search?q=FOO',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { results: Record<string, unknown[]> };
    expect((body.results.pools as unknown[]).length).toBeGreaterThan(0);
  });

  it('requires the q query parameter', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/search',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(r.statusCode).toBe(400);
  });

  it('caches results on repeated queries', async () => {
    const first = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/search?q=cache-probe-xyz',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(first.statusCode).toBe(200);
    expect(first.headers['x-cache']).toBe('miss');

    const second = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/search?q=cache-probe-xyz',
      headers: { cookie: cookieFor(h.built, adminSid) },
    });
    expect(second.statusCode).toBe(200);
    expect(second.headers['x-cache']).toBe('hit');
  });

  it('still responds (with empty groups for forbidden kinds) for viewers', async () => {
    const r = await h.built.app.inject({
      method: 'GET',
      url: '/api/v1/search?q=foo',
      headers: { cookie: cookieFor(h.built, viewerSid) },
    });
    // Viewer can read all kinds per authz.canRead; just ensure 200.
    expect(r.statusCode).toBe(200);
  });
});
