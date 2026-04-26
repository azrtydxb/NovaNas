import { randomUUID } from 'node:crypto';
import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { Job } from '@novanas/db';
import pino from 'pino';
import { afterAll, beforeAll, describe, expect, it } from 'vitest';
import { type BuiltApp, buildApp } from '../app.js';
import { AuthzRole } from '../auth/authz.js';
import {
  FakeCustomObjectsApi,
  cookieFor,
  fakeKeycloak,
  fakeRedis,
  testEnv,
} from '../resources/_test-helpers.js';
import type { DbClient } from '../services/db.js';

function fakeDb(): DbClient {
  const rows: Job[] = [];
  function sel() {
    const c = {
      from() {
        return c;
      },
      where() {
        return c;
      },
      orderBy() {
        return c;
      },
      limit() {
        return c;
      },
      // biome-ignore lint/suspicious/noThenProperty: drizzle thenable
      then<T>(r: (v: Job[]) => T) {
        return Promise.resolve(rows).then(r);
      },
    };
    return c;
  }
  const db = {
    select() {
      return sel();
    },
    insert() {
      return {
        values(v: Partial<Job>) {
          const row: Job = {
            id: randomUUID(),
            kind: v.kind ?? 'x',
            state: 'queued',
            progressPercent: 0,
            startedAt: null,
            finishedAt: null,
            params: (v.params as Record<string, unknown>) ?? {},
            result: null,
            error: null,
            ownerId: v.ownerId ?? null,
            createdAt: new Date(),
            updatedAt: new Date(),
          };
          rows.push(row);
          return { returning: () => Promise.resolve([row]) };
        },
      };
    },
    update() {
      const c = {
        set() {
          return c;
        },
        where() {
          return c;
        },
        returning: () => Promise.resolve([]),
      };
      return c;
    },
  };
  return db as unknown as DbClient;
}

describe('snapshot action routes (restore)', () => {
  let built: BuiltApp;
  let adminSid: string;

  beforeAll(async () => {
    const kube = new FakeCustomObjectsApi();
    await kube.seed('snapshots', {
      apiVersion: 'novanas.io/v1alpha1',
      kind: 'Snapshot',
      metadata: { name: 'snap1' },
      spec: { source: 'pool/ds' },
    });
    built = await buildApp({
      env: testEnv,
      logger: pino({ level: 'silent' }),
      redis: fakeRedis(),
      keycloak: fakeKeycloak(),
      kubeCustom: kube as unknown as CustomObjectsApi,
      disableSwagger: true,
      disablePubSub: true,
      db: fakeDb(),
    });
    adminSid = await built.sessions.create({
      userId: 'a',
      username: 'admin',
      createdAt: Date.now(),
      expiresAt: Date.now() + 3600_000,
      idToken: 't',
      accessToken: 't',
      claims: { sub: 'a', preferred_username: 'admin', realm_access: { roles: [AuthzRole.Admin] } },
    });
  });
  afterAll(async () => built.app.close());

  it('POST /restore returns job id', async () => {
    const r = await built.app.inject({
      method: 'POST',
      url: '/api/v1/snapshots/snap1/restore',
      headers: { cookie: cookieFor(built, adminSid), 'content-type': 'application/json' },
      payload: { targetVolume: 'pool/restored' },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { jobId?: string };
    expect(body.jobId).toBeTruthy();
  });

  it('400 on missing body', async () => {
    const r = await built.app.inject({
      method: 'POST',
      url: '/api/v1/snapshots/snap1/restore',
      headers: { cookie: cookieFor(built, adminSid), 'content-type': 'application/json' },
      payload: {},
    });
    expect(r.statusCode).toBe(400);
  });
});
