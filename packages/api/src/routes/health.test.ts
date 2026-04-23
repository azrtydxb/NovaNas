import Fastify from 'fastify';
import type { Redis } from 'ioredis';
import { afterEach, describe, expect, it } from 'vitest';
import { fakeRedis } from '../resources/_test-helpers.js';
import type { DbClient } from '../services/db.js';
import { healthRoutes } from './health.js';

function makeApp(deps: { redis: Redis; db?: DbClient | null; timeoutMs?: number }) {
  const app = Fastify();
  app.register(async (s) => healthRoutes(s, deps));
  return app;
}

describe('healthRoutes', () => {
  let app: ReturnType<typeof Fastify>;

  afterEach(async () => {
    if (app) await app.close();
  });

  it('livez always returns 200', async () => {
    app = makeApp({ redis: fakeRedis() });
    const res = await app.inject({ method: 'GET', url: '/livez' });
    expect(res.statusCode).toBe(200);
    expect(res.body).toBe('ok');
  });

  it('health returns uptime', async () => {
    app = makeApp({ redis: fakeRedis() });
    const res = await app.inject({ method: 'GET', url: '/health' });
    expect(res.statusCode).toBe(200);
    const body = JSON.parse(res.body);
    expect(body.status).toBe('ok');
    expect(typeof body.uptime).toBe('number');
  });

  it('readyz is 200 when redis pings and db is absent', async () => {
    app = makeApp({ redis: fakeRedis() });
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(200);
    const body = JSON.parse(res.body);
    expect(body.status).toBe('ready');
    expect(body.components.redis.status).toBe('ok');
    expect(body.components.db.status).toBe('skipped');
  });

  it('readyz is 503 when redis ping fails', async () => {
    const broken = {
      ...fakeRedis(),
      async ping() {
        throw new Error('conn refused');
      },
    } as unknown as Redis;
    app = makeApp({ redis: broken });
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(503);
    const body = JSON.parse(res.body);
    expect(body.status).toBe('unready');
    expect(body.components.redis.status).toBe('down');
    expect(body.components.redis.error).toBe('conn refused');
  });

  it('readyz is 503 when redis times out', async () => {
    const slow = {
      ...fakeRedis(),
      async ping() {
        await new Promise((r) => setTimeout(r, 200));
        return 'PONG';
      },
    } as unknown as Redis;
    app = makeApp({ redis: slow, timeoutMs: 20 });
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(503);
    const body = JSON.parse(res.body);
    expect(body.components.redis.status).toBe('down');
    expect(body.components.redis.error).toContain('timeout');
  });

  it('readyz checks db when provided', async () => {
    const db = {
      async execute() {
        return [];
      },
    } as unknown as DbClient;
    app = makeApp({ redis: fakeRedis(), db });
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(200);
    const body = JSON.parse(res.body);
    expect(body.components.db.status).toBe('ok');
  });

  it('readyz is 503 when db query fails', async () => {
    const db = {
      async execute() {
        throw new Error('db down');
      },
    } as unknown as DbClient;
    app = makeApp({ redis: fakeRedis(), db });
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(503);
    const body = JSON.parse(res.body);
    expect(body.components.db.status).toBe('down');
    expect(body.components.db.error).toBe('db down');
  });

  it('readyz rejects unexpected redis reply', async () => {
    const weird = {
      ...fakeRedis(),
      async ping() {
        return 'what';
      },
    } as unknown as Redis;
    app = makeApp({ redis: weird });
    const res = await app.inject({ method: 'GET', url: '/readyz' });
    expect(res.statusCode).toBe(503);
    const body = JSON.parse(res.body);
    expect(body.components.redis.status).toBe('down');
    expect(body.components.redis.error).toContain('unexpected reply');
  });
});
