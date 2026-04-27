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

/** Minimal in-memory DB that mimics the shape JobsService needs. */
function fakeDb(): { db: DbClient; rows: Job[] } {
  const rows: Job[] = [];
  function buildSelectChain() {
    const chain = {
      from() {
        return chain;
      },
      where() {
        return chain;
      },
      orderBy() {
        return chain;
      },
      limit() {
        return chain;
      },
      // biome-ignore lint/suspicious/noThenProperty: mimicking Drizzle's thenable query builder.
      then<T>(resolve: (v: Job[]) => T) {
        return Promise.resolve(rows.slice()).then(resolve);
      },
    };
    return chain;
  }
  const db = {
    select() {
      return buildSelectChain();
    },
    insert() {
      return {
        values(v: Partial<Job>) {
          const row: Job = {
            id: randomUUID(),
            kind: v.kind ?? 'unknown',
            state: v.state ?? 'queued',
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
      const chain = {
        set() {
          return chain;
        },
        where() {
          return chain;
        },
        returning: () => Promise.resolve(rows.length > 0 ? [rows[0]!] : []),
      };
      return chain;
    },
  };
  return { db: db as unknown as DbClient, rows };
}

describe('system action routes (E1-API-Actions)', () => {
  let built: BuiltApp;
  let adminSid: string;
  const fake = fakeDb();

  beforeAll(async () => {
    built = await buildApp({
      env: testEnv,
      logger: pino({ level: 'silent' }),
      redis: fakeRedis(),
      keycloak: fakeKeycloak(),
      kubeCustom: new FakeCustomObjectsApi() as unknown as CustomObjectsApi,
      disableSwagger: true,
      disablePubSub: true,
      db: fake.db,
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

  it('POST /system/reset returns job id', async () => {
    const r = await built.app.inject({
      method: 'POST',
      url: '/api/v1/system/reset?tier=soft',
      headers: { cookie: cookieFor(built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { accepted: boolean; jobId?: string; status: string };
    expect(body.accepted).toBe(true);
    expect(body.jobId).toBeTruthy();
    expect(body.status).toBe('pending');
  });

  it('POST /system/reset rejects invalid tier', async () => {
    const r = await built.app.inject({
      method: 'POST',
      url: '/api/v1/system/reset?tier=bogus',
      headers: { cookie: cookieFor(built, adminSid) },
    });
    expect(r.statusCode).toBe(400);
  });

  it('POST /system/support-bundle returns jobId + downloadUrl', async () => {
    const r = await built.app.inject({
      method: 'POST',
      url: '/api/v1/system/support-bundle',
      headers: { cookie: cookieFor(built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { jobId: string; downloadUrl: string };
    expect(body.jobId).toBeTruthy();
    expect(body.downloadUrl).toContain(body.jobId);
  });

  it('POST /system/check-update returns 200', async () => {
    const r = await built.app.inject({
      method: 'POST',
      url: '/api/v1/system/check-update',
      headers: { cookie: cookieFor(built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
  });
});
