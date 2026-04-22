import { describe, expect, it, vi } from 'vitest';
import type { DbClient } from './db.js';
import { startRollupGc } from './rollup-gc.js';

function silentLogger() {
  return {
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    debug: vi.fn(),
    trace: vi.fn(),
    fatal: vi.fn(),
    level: 'silent',
    silent: vi.fn(),
    child: vi.fn(),
  } as unknown as import('fastify').FastifyBaseLogger;
}

/**
 * Build a fake `DbClient` whose `.delete(...).where(...).returning()` chain
 * returns a scripted batch of ids on each successive call. This lets us
 * verify that `startRollupGc` issues exactly three delete statements (one
 * per retention tier) and reports the accumulated row count.
 */
function fakeDbReturning(batches: Array<Array<{ id: string }>>): DbClient {
  let call = 0;
  return {
    delete() {
      const idx = call++;
      return {
        where() {
          return {
            async returning() {
              return batches[idx] ?? [];
            },
          };
        },
      };
    },
  } as unknown as DbClient;
}

describe('startRollupGc', () => {
  it('issues three delete statements (5m, 1h, daily catch-all) and sums deletions', async () => {
    const db = fakeDbReturning([
      [{ id: 'a' }, { id: 'b' }], // 5m tier
      [{ id: 'c' }], // hourly tier
      [{ id: 'd' }, { id: 'e' }, { id: 'f' }], // daily long-tail
    ]);
    const h = startRollupGc({
      db,
      logger: silentLogger(),
      intervalMs: 1_000_000,
      runOnStart: false,
    });
    const r = await h.runOnce();
    h.stop();
    expect(r.deleted).toBe(6);
  });

  it('honours custom retention windows without throwing', async () => {
    const db = fakeDbReturning([[], [], []]);
    const h = startRollupGc({
      db,
      logger: silentLogger(),
      intervalMs: 1_000_000,
      runOnStart: false,
      retention: { fiveMinuteDays: 7, hourlyDays: 14, dailyDays: 30 },
    });
    const r = await h.runOnce();
    h.stop();
    expect(r.deleted).toBe(0);
  });

  it('no-ops when db is null', async () => {
    const h = startRollupGc({ db: null, logger: silentLogger(), intervalMs: 1_000_000 });
    const r = await h.runOnce();
    h.stop();
    expect(r.deleted).toBe(0);
  });

  it('swallows db errors (logs only)', async () => {
    const db = {
      delete() {
        return {
          where() {
            return {
              async returning() {
                throw new Error('boom');
              },
            };
          },
        };
      },
    } as unknown as DbClient;
    const logger = silentLogger();
    const h = startRollupGc({ db, logger, intervalMs: 1_000_000 });
    const r = await h.runOnce();
    h.stop();
    expect(r.deleted).toBe(0);
    expect(logger.error).toHaveBeenCalled();
  });
});
