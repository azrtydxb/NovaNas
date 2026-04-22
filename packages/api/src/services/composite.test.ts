import { describe, expect, it, vi } from 'vitest';
import {
  type CompositeStep,
  type OrphanSweeperAdapter,
  type RetryConfig,
  retryWithBackoff,
  runComposite,
  startOrphanSweeper,
} from './composite.js';

/** Injected no-op sleep so tests don't wait real wall-clock time. */
const fastRetry: RetryConfig = { sleep: async () => {} };

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

describe('runComposite', () => {
  it('runs all steps and returns success', async () => {
    const ctx = { log: [] as string[] };
    const steps: CompositeStep<typeof ctx>[] = [
      { name: 'a', exec: async (c) => c.log.push('a') },
      { name: 'b', exec: async (c) => c.log.push('b') },
    ];
    const r = await runComposite({ ctx, steps, rollbackRetry: fastRetry });
    expect(r.success).toBe(true);
    if (r.success) expect(r.completed.map((s) => s.name)).toEqual(['a', 'b']);
    expect(ctx.log).toEqual(['a', 'b']);
  });

  it('rolls back completed steps in reverse on failure (first-attempt success)', async () => {
    const ctx = { log: [] as string[] };
    const steps: CompositeStep<typeof ctx>[] = [
      {
        name: 'a',
        exec: async (c) => {
          c.log.push('a-exec');
          return 'a-result';
        },
        rollback: async (c) => {
          c.log.push('a-rollback');
        },
      },
      {
        name: 'b',
        exec: async (c) => {
          c.log.push('b-exec');
          return 'b-result';
        },
        rollback: async (c) => {
          c.log.push('b-rollback');
        },
      },
      {
        name: 'c',
        exec: async () => {
          throw new Error('boom');
        },
      },
    ];
    const r = await runComposite({ ctx, steps, rollbackRetry: fastRetry });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.failedStep).toBe('c');
      expect(r.error.message).toBe('boom');
      expect(r.rolledBack).toEqual(['b', 'a']);
    }
    expect(ctx.log).toEqual(['a-exec', 'b-exec', 'b-rollback', 'a-rollback']);
  });

  it('retries rollback with exponential backoff and succeeds on third attempt', async () => {
    const sleeps: number[] = [];
    const sleep = vi.fn(async (ms: number) => {
      sleeps.push(ms);
    });
    let rbCalls = 0;
    const steps: CompositeStep<object>[] = [
      {
        name: 'a',
        exec: async () => 'r',
        rollback: async () => {
          rbCalls++;
          if (rbCalls < 3) throw new Error(`try ${rbCalls}`);
        },
      },
      {
        name: 'b',
        exec: async () => {
          throw new Error('boom');
        },
      },
    ];
    const r = await runComposite({
      ctx: {},
      steps,
      rollbackRetry: { attempts: 3, baseDelayMs: 1000, sleep },
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.rolledBack).toEqual(['a']);
      expect(r.rollbackErrors).toHaveLength(0);
      expect(r.orphaned).toEqual([]);
    }
    expect(rbCalls).toBe(3);
    expect(sleeps).toEqual([1000, 2000]);
  });

  it('terminal rollback failure stamps orphan annotation', async () => {
    const annotateOrphan = vi.fn(async (_info: import('./composite.js').OrphanInfo) => {});
    const steps: CompositeStep<object>[] = [
      {
        name: 'a',
        exec: async () => 'r',
        rollback: async () => {
          throw new Error('rbfail');
        },
      },
      {
        name: 'b',
        exec: async () => {
          throw new Error('boom');
        },
      },
    ];
    const r = await runComposite({
      ctx: {},
      steps,
      rollbackRetry: fastRetry,
      annotateOrphan,
    });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.rollbackErrors).toHaveLength(1);
      expect(r.rollbackErrors[0]?.step).toBe('a');
      expect(r.orphaned).toEqual(['a']);
    }
    expect(annotateOrphan).toHaveBeenCalledOnce();
    const info = annotateOrphan.mock.calls[0]?.[0] as
      | { step: string; timestamp: string }
      | undefined;
    expect(info?.step).toBe('a');
    expect(info?.timestamp).toMatch(/\d{4}-\d{2}-\d{2}T/);
  });
});

describe('retryWithBackoff', () => {
  it('returns the value on first success without sleeping', async () => {
    const sleep = vi.fn(async () => {});
    const out = await retryWithBackoff(async () => 'ok', { sleep });
    expect(out).toBe('ok');
    expect(sleep).not.toHaveBeenCalled();
  });

  it('rethrows the final error after attempts exhausted', async () => {
    const sleep = vi.fn(async () => {});
    await expect(
      retryWithBackoff(
        async () => {
          throw new Error('nope');
        },
        { attempts: 3, baseDelayMs: 10, sleep }
      )
    ).rejects.toThrow('nope');
    expect(sleep).toHaveBeenCalledTimes(2);
  });
});

describe('OrphanSweeper', () => {
  function makeAdapter(overrides: Partial<OrphanSweeperAdapter> = {}): OrphanSweeperAdapter {
    return {
      list: async () => [],
      cleanup: async () => {},
      markAbandoned: async () => {},
      ...overrides,
    };
  }

  it('re-attempts cleanup on known orphans', async () => {
    const list = vi.fn(async () => [
      {
        id: 'Dataset/foo',
        kind: 'Dataset',
        name: 'foo',
        annotatedAt: new Date().toISOString(),
      },
    ]);
    const cleanup = vi.fn(async () => {});
    const markAbandoned = vi.fn(async () => {});
    const adapter = makeAdapter({ list, cleanup, markAbandoned });

    const h = startOrphanSweeper({
      adapter,
      logger: silentLogger(),
      intervalMs: 1_000_000,
      runOnStart: false,
    });
    const r = await h.runOnce();
    h.stop();

    expect(cleanup).toHaveBeenCalledOnce();
    expect(markAbandoned).not.toHaveBeenCalled();
    expect(r.cleaned).toEqual(['Dataset/foo']);
  });

  it('graduates stale orphans to abandoned after 7 days', async () => {
    const eightDaysAgo = new Date(Date.now() - 8 * 24 * 3600 * 1000).toISOString();
    const list = vi.fn(async () => [
      { id: 'Share/bar', kind: 'Share', name: 'bar', annotatedAt: eightDaysAgo },
    ]);
    const cleanup = vi.fn(async () => {
      throw new Error('still failing');
    });
    const markAbandoned = vi.fn(async () => {});
    const adapter = makeAdapter({ list, cleanup, markAbandoned });

    const h = startOrphanSweeper({
      adapter,
      logger: silentLogger(),
      intervalMs: 1_000_000,
      runOnStart: false,
      abandonAfterDays: 7,
    });
    const r = await h.runOnce();
    h.stop();

    expect(cleanup).toHaveBeenCalledOnce();
    expect(markAbandoned).toHaveBeenCalledOnce();
    expect(r.abandoned).toEqual(['Share/bar']);
  });

  it('keeps fresh orphans in the queue when cleanup still fails', async () => {
    const list = vi.fn(async () => [
      {
        id: 'Disk/baz',
        kind: 'Disk',
        name: 'baz',
        annotatedAt: new Date().toISOString(),
      },
    ]);
    const cleanup = vi.fn(async () => {
      throw new Error('retry later');
    });
    const markAbandoned = vi.fn(async () => {});
    const adapter = makeAdapter({ list, cleanup, markAbandoned });

    const h = startOrphanSweeper({
      adapter,
      logger: silentLogger(),
      intervalMs: 1_000_000,
      runOnStart: false,
    });
    const r = await h.runOnce();
    h.stop();

    expect(markAbandoned).not.toHaveBeenCalled();
    expect(r.cleaned).toEqual([]);
    expect(r.abandoned).toEqual([]);
  });
});
