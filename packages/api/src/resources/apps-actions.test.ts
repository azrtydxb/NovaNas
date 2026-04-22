import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { AuthzRole } from '../auth/authz.js';
import { type TestAppHandle, buildTestApp, cookieFor } from './_test-helpers.js';

function sampleFor(user: string, name: string) {
  return {
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'AppInstance',
    metadata: { name, namespace: `user-${user}` },
    spec: { app: 'jellyfin', version: '10.8.0' },
  };
}

describe('apps action routes (E1-API-Actions)', () => {
  let h: TestAppHandle;
  let aliceSid: string;
  let viewerSid: string;

  beforeAll(async () => {
    h = await buildTestApp();
    h.kube.seed('appinstances', sampleFor('alice', 'jelly'), 'user-alice');
    aliceSid = await h.authAs({ username: 'alice', roles: [AuthzRole.User] });
    viewerSid = await h.authAs({ username: 'obs', roles: [AuthzRole.Viewer] });
  });
  afterAll(async () => h.built.app.close());

  it('POST /start returns 200 with action response', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/apps/user-alice/jelly/start',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { accepted: boolean; status: string };
    expect(body.accepted).toBe(true);
    expect(body.status).toBe('running');
  });

  it('POST /stop returns 200', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/apps/user-alice/jelly/stop',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(r.statusCode).toBe(200);
  });

  it('POST /update requires version in body', async () => {
    const bad = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/apps/user-alice/jelly/update',
      headers: { cookie: cookieFor(h.built, aliceSid), 'content-type': 'application/json' },
      payload: {},
    });
    expect(bad.statusCode).toBe(400);

    const good = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/apps/user-alice/jelly/update',
      headers: { cookie: cookieFor(h.built, aliceSid), 'content-type': 'application/json' },
      payload: { version: '10.9.0' },
    });
    expect(good.statusCode).toBe(200);
  });

  it('viewer gets 403 on actions', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/apps/user-alice/jelly/start',
      headers: { cookie: cookieFor(h.built, viewerSid) },
    });
    expect(r.statusCode).toBe(403);
  });

  it('404 when target does not exist', async () => {
    const r = await h.built.app.inject({
      method: 'POST',
      url: '/api/v1/apps/user-alice/ghost/start',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(r.statusCode).toBe(404);
  });

  it('DELETE with deleteData=true requires confirm', async () => {
    const noConfirm = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/apps/user-alice/jelly?deleteData=true',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(noConfirm.statusCode).toBe(400);

    const confirmed = await h.built.app.inject({
      method: 'DELETE',
      url: '/api/v1/apps/user-alice/jelly?deleteData=true&confirm=true',
      headers: { cookie: cookieFor(h.built, aliceSid) },
    });
    expect(confirmed.statusCode).toBe(200);
  });
});
