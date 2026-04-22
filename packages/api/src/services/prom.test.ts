import type { Redis } from 'ioredis';
import { describe, expect, it, vi } from 'vitest';
import type { Env } from '../env.js';
import { createPromClient } from './prom.js';

const env = { PROMETHEUS_URL: 'http://prom.test' } as unknown as Env;

function okJson(body: unknown): Response {
  return {
    ok: true,
    status: 200,
    json: async () => body,
  } as unknown as Response;
}

describe('createPromClient', () => {
  it('queries prometheus range and returns typed series', async () => {
    const fetchImpl = vi.fn().mockResolvedValue(
      okJson({
        status: 'success',
        data: {
          resultType: 'matrix',
          result: [
            {
              metric: { pool: 'a' },
              values: [
                [100, '1'],
                [160, '2'],
              ],
            },
          ],
        },
      })
    );
    const c = createPromClient(env, { fetchImpl: fetchImpl as unknown as typeof fetch });
    const out = await c.queryRange('up', {
      start: new Date(100_000),
      end: new Date(200_000),
      stepSeconds: 30,
    });
    expect(out).toHaveLength(1);
    expect(out[0]!.labels.pool).toBe('a');
    expect(out[0]!.points).toEqual([
      { t: 100, v: 1 },
      { t: 160, v: 2 },
    ]);
    const url = fetchImpl.mock.calls[0]![0] as string;
    expect(url).toContain('/api/v1/query_range');
    expect(url).toContain('query=up');
  });

  it('caches results in Redis', async () => {
    const redisStore = new Map<string, string>();
    const redis = {
      async get(k: string) {
        return redisStore.get(k) ?? null;
      },
      async setex(k: string, _ttl: number, v: string) {
        redisStore.set(k, v);
        return 'OK';
      },
    } as unknown as Redis;
    const fetchImpl = vi
      .fn()
      .mockResolvedValue(okJson({ status: 'success', data: { resultType: 'matrix', result: [] } }));
    const c = createPromClient(env, { fetchImpl: fetchImpl as unknown as typeof fetch, redis });
    const range = { start: new Date(), end: new Date(), stepSeconds: 30 };
    await c.query('up', range);
    await c.query('up', range);
    expect(fetchImpl).toHaveBeenCalledTimes(1);
  });

  it('throws when PROMETHEUS_URL missing', async () => {
    const c = createPromClient({ PROMETHEUS_URL: undefined } as unknown as Env, {
      fetchImpl: vi.fn() as unknown as typeof fetch,
    });
    await expect(c.queryInstant('up')).rejects.toThrow(/prometheus_url_not_configured/);
  });
});
