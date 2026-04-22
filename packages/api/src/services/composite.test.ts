import { describe, expect, it } from 'vitest';
import { type CompositeStep, runComposite } from './composite.js';

describe('runComposite', () => {
  it('runs all steps and returns success', async () => {
    const ctx = { log: [] as string[] };
    const steps: CompositeStep<typeof ctx>[] = [
      { name: 'a', exec: async (c) => c.log.push('a') },
      { name: 'b', exec: async (c) => c.log.push('b') },
    ];
    const r = await runComposite({ ctx, steps });
    expect(r.success).toBe(true);
    if (r.success) expect(r.completed.map((s) => s.name)).toEqual(['a', 'b']);
    expect(ctx.log).toEqual(['a', 'b']);
  });

  it('rolls back completed steps in reverse on failure', async () => {
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
    const r = await runComposite({ ctx, steps });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.failedStep).toBe('c');
      expect(r.error.message).toBe('boom');
      expect(r.rolledBack).toEqual(['b', 'a']);
    }
    expect(ctx.log).toEqual(['a-exec', 'b-exec', 'b-rollback', 'a-rollback']);
  });

  it('tolerates rollback errors and records them', async () => {
    const ctx = {};
    const steps: CompositeStep<typeof ctx>[] = [
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
    const r = await runComposite({ ctx, steps });
    expect(r.success).toBe(false);
    if (!r.success) {
      expect(r.rollbackErrors).toHaveLength(1);
      expect(r.rollbackErrors[0]?.step).toBe('a');
    }
  });
});
