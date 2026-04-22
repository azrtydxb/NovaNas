import type { FastifyBaseLogger } from 'fastify';

/**
 * Small transactional executor for multi-CRD composite operations.
 *
 * Each step has an `exec` and optional `rollback`. If any step fails, the
 * executor runs `rollback` for every previously-completed step in reverse
 * order (best-effort; rollback errors are logged but do not mask the
 * original failure).
 */

export interface CompositeStep<Ctx, Result = unknown> {
  name: string;
  exec: (ctx: Ctx) => Promise<Result>;
  rollback?: (ctx: Ctx, result: Result) => Promise<void> | void;
}

export interface CompositeOptions<Ctx> {
  ctx: Ctx;
  steps: CompositeStep<Ctx>[];
  logger?: FastifyBaseLogger;
}

export interface CompletedStep<Result = unknown> {
  name: string;
  result: Result;
}

export interface CompositeSuccess<Result = unknown> {
  success: true;
  completed: CompletedStep<Result>[];
}

export interface CompositeFailure {
  success: false;
  completed: CompletedStep[];
  failedStep: string;
  error: Error;
  rolledBack: string[];
  rollbackErrors: { step: string; error: Error }[];
}

export type CompositeResult = CompositeSuccess | CompositeFailure;

export async function runComposite<Ctx>(opts: CompositeOptions<Ctx>): Promise<CompositeResult> {
  const { ctx, steps, logger } = opts;
  const completed: CompletedStep[] = [];

  for (const step of steps) {
    try {
      const result = await step.exec(ctx);
      completed.push({ name: step.name, result });
      logger?.info({ step: step.name }, 'composite.step.success');
    } catch (err) {
      const error = err instanceof Error ? err : new Error(String(err));
      logger?.error({ step: step.name, err: error }, 'composite.step.failed');
      const rolledBack: string[] = [];
      const rollbackErrors: { step: string; error: Error }[] = [];
      // rollback in reverse order
      for (let i = completed.length - 1; i >= 0; i--) {
        const done = completed[i]!;
        const original = steps.find((s) => s.name === done.name);
        if (!original?.rollback) continue;
        try {
          await original.rollback(ctx, done.result);
          rolledBack.push(done.name);
          logger?.info({ step: done.name }, 'composite.rollback.success');
        } catch (rbErr) {
          const rollbackError = rbErr instanceof Error ? rbErr : new Error(String(rbErr));
          rollbackErrors.push({ step: done.name, error: rollbackError });
          logger?.error({ step: done.name, err: rollbackError }, 'composite.rollback.failed');
        }
      }
      return {
        success: false,
        completed,
        failedStep: step.name,
        error,
        rolledBack,
        rollbackErrors,
      };
    }
  }

  return { success: true, completed };
}
