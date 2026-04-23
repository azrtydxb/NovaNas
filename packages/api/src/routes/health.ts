import { sql } from 'drizzle-orm';
import type { FastifyInstance } from 'fastify';
import type { Redis } from 'ioredis';
import type { DbClient } from '../services/db.js';

export interface HealthDeps {
  redis: Redis;
  db?: DbClient | null;
  /**
   * Max time to wait for each backing service to respond before marking it
   * unready. Kubernetes readiness probes expect low-latency answers.
   */
  timeoutMs?: number;
}

interface ComponentStatus {
  status: 'ok' | 'down';
  error?: string;
  latencyMs?: number;
}

async function checkRedis(redis: Redis, timeoutMs: number): Promise<ComponentStatus> {
  const started = Date.now();
  try {
    const result = await Promise.race<string>([
      redis.ping(),
      new Promise<string>((_, reject) =>
        setTimeout(() => reject(new Error('redis ping timeout')), timeoutMs)
      ),
    ]);
    if (result !== 'PONG') {
      return { status: 'down', error: `unexpected reply: ${result}` };
    }
    return { status: 'ok', latencyMs: Date.now() - started };
  } catch (err) {
    return { status: 'down', error: err instanceof Error ? err.message : String(err) };
  }
}

async function checkDb(db: DbClient, timeoutMs: number): Promise<ComponentStatus> {
  const started = Date.now();
  try {
    await Promise.race([
      db.execute(sql`SELECT 1`),
      new Promise((_, reject) =>
        setTimeout(() => reject(new Error('db query timeout')), timeoutMs)
      ),
    ]);
    return { status: 'ok', latencyMs: Date.now() - started };
  } catch (err) {
    return { status: 'down', error: err instanceof Error ? err.message : String(err) };
  }
}

export async function healthRoutes(app: FastifyInstance, deps: HealthDeps): Promise<void> {
  const timeoutMs = deps.timeoutMs ?? 1500;

  app.get(
    '/health',
    {
      schema: {
        description: 'Liveness + readiness combined probe.',
        tags: ['system'],
        response: {
          200: {
            type: 'object',
            properties: {
              status: { type: 'string' },
              uptime: { type: 'number' },
            },
            required: ['status', 'uptime'],
          },
        },
      },
    },
    async () => ({ status: 'ok', uptime: process.uptime() })
  );

  app.get(
    '/livez',
    { schema: { description: 'Kubernetes liveness probe.', tags: ['system'] } },
    async (_req, reply) => reply.code(200).send('ok')
  );

  app.get(
    '/readyz',
    { schema: { description: 'Kubernetes readiness probe.', tags: ['system'] } },
    async (_req, reply) => {
      const [redisStatus, dbStatus] = await Promise.all([
        checkRedis(deps.redis, timeoutMs),
        deps.db ? checkDb(deps.db, timeoutMs) : Promise.resolve<ComponentStatus>({ status: 'ok' }),
      ]);
      const allOk = redisStatus.status === 'ok' && dbStatus.status === 'ok';
      const body = {
        status: allOk ? 'ready' : 'unready',
        components: {
          redis: redisStatus,
          db: deps.db ? dbStatus : { status: 'skipped' as const },
        },
      };
      return reply.code(allOk ? 200 : 503).send(body);
    }
  );
}
