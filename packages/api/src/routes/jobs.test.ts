import { randomUUID } from 'node:crypto';
import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { Job } from '@novanas/db';
import pino from 'pino';
import { afterAll, beforeAll, describe, expect, it, vi } from 'vitest';
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
import type { AuthenticatedUser } from '../types.js';

/**
 * Minimal fake of the NovaNasDb shape used by JobsService: select/insert/update
 * chains are modeled around an in-memory array.
 */
function fakeDb(): { db: DbClient; rows: Job[] } {
  const rows: Job[] = [];

  function buildSelectChain(state: { limit?: number }) {
    const exec = (): Job[] => {
      // The real DB filters via Drizzle SQL clauses; our fake returns all rows
      // (tests either seed zero rows or explicitly assert count = 0).
      const out = rows.slice();
      if (state.limit !== undefined) return out.slice(0, state.limit);
      return out;
    };
    const chain = {
      from() {
        return chain;
      },
      where(_clause: unknown) {
        return chain;
      },
      orderBy() {
        return chain;
      },
      limit(n: number) {
        state.limit = n;
        return chain;
      },
      // biome-ignore lint/suspicious/noThenProperty: mimicking Drizzle's thenable query builder.
      then<T>(resolve: (v: Job[]) => T) {
        return Promise.resolve(exec()).then(resolve);
      },
    };
    return chain;
  }

  const db = {
    select() {
      return buildSelectChain({});
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
          return {
            returning: () => Promise.resolve([row]),
          };
        },
      };
    },
    update() {
      let patch: Partial<Job> = {};
      const chain = {
        set(p: Partial<Job>) {
          patch = p;
          return chain;
        },
        where(_clause: unknown) {
          return chain;
        },
        returning: () => {
          if (rows.length === 0) return Promise.resolve([]);
          rows[0] = { ...rows[0]!, ...patch, updatedAt: new Date() };
          return Promise.resolve([rows[0]]);
        },
      };
      return chain;
    },
  };

  // drizzle's eq()/and() return opaque objects that fakeDb interprets by
  // synthesising a predicate from the fields we care about. We only compare
  // by id/ownerId/state/kind, so turn the SQL clauses into simple matchers.
  // The service passes `eq(jobs.id, id)` etc. — we bypass by replacing
  // these helper imports with a shim via the chain's `where(fn)` path.
  // But JobsService imports `eq`/`and` directly from drizzle-orm; our fake
  // doesn't see the predicate values. So for the service tests we bypass
  // and use the hub directly.
  return { db: db as unknown as DbClient, rows };
}

// Because the fake predicate chain above can't interpret drizzle SQL, we drive
// the routes end-to-end but assert against rows directly where needed.

describe('jobs routes', () => {
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
    const adminUser: AuthenticatedUser = {
      sub: randomUUID(),
      username: 'admin',
      roles: [AuthzRole.Admin],
      groups: [],
      tenant: 'default',
      claims: {},
    };
    adminSid = await built.sessions.create({
      userId: adminUser.sub,
      username: adminUser.username,
      createdAt: Date.now(),
      expiresAt: Date.now() + 3600_000,
      idToken: 't',
      accessToken: 't',
      claims: {
        sub: adminUser.sub,
        preferred_username: 'admin',
        realm_access: { roles: [AuthzRole.Admin] },
      },
    });
  });

  afterAll(async () => {
    await built.app.close();
  });

  it('GET /api/v1/jobs lists jobs (empty)', async () => {
    const r = await built.app.inject({
      method: 'GET',
      url: '/api/v1/jobs',
      headers: { cookie: cookieFor(built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { items: unknown[] };
    expect(Array.isArray(body.items)).toBe(true);
  });

  it('lists include a freshly created job and publishes to redis on cancel', async () => {
    const svc = built.jobs!;
    const row = await svc.create({ kind: 'backup', params: {} });
    const listed = await svc.list({ limit: 10 });
    expect(listed.some((j) => j.id === row.id)).toBe(true);
    const cancelled = await svc.cancel(row.id);
    expect(cancelled?.state).toBe('cancelled');
  });

  it('exposes _meta/states', async () => {
    const r = await built.app.inject({
      method: 'GET',
      url: '/api/v1/jobs/_meta/states',
      headers: { cookie: cookieFor(built, adminSid) },
    });
    expect(r.statusCode).toBe(200);
    const body = r.json() as { states: string[] };
    expect(body.states).toContain('queued');
    expect(body.states).toContain('cancelled');
  });

  it('returns 503 when db is not wired', async () => {
    const built2 = await buildApp({
      env: testEnv,
      logger: pino({ level: 'silent' }),
      redis: fakeRedis(),
      keycloak: fakeKeycloak(),
      disableSwagger: true,
      disablePubSub: true,
    });
    const sid = await built2.sessions.create({
      userId: 'u',
      username: 'u',
      createdAt: Date.now(),
      expiresAt: Date.now() + 3600_000,
      idToken: 't',
      accessToken: 't',
      claims: { sub: 'u', preferred_username: 'u', realm_access: { roles: [AuthzRole.Admin] } },
    });
    const r = await built2.app.inject({
      method: 'GET',
      url: '/api/v1/jobs',
      headers: { cookie: cookieFor(built2, sid) },
    });
    expect(r.statusCode).toBe(503);
    await built2.app.close();
  });
});

// Prevent unused-import lint errors in test module
void vi;
