import type { FastifyBaseLogger } from 'fastify';

/**
 * Small transactional executor for multi-CRD composite operations.
 *
 * Each step has an `exec` and optional `rollback`. If any step fails, the
 * executor runs `rollback` for every previously-completed step in reverse
 * order. Rollback is retried with exponential backoff (3 attempts at
 * 1s / 2s / 4s) to paper over transient 409/5xx errors from the kube API.
 *
 * On terminal rollback failure we surface a `rollbackErrors` entry and (if
 * the caller provides an `annotateOrphan` hook) stamp the primary resource
 * with `novanas.io/rollback-orphan: <ISO8601>` so the {@link OrphanSweeper}
 * can re-try cleanup later.
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
  /** Retry config for rollback steps. Defaults: 3 attempts, 1s/2s/4s. */
  rollbackRetry?: RetryConfig;
  /**
   * Invoked when rollback exhausts its retries. Implementations typically
   * annotate the failed resource so the OrphanSweeper can take a second
   * pass. Errors inside this hook are logged and swallowed.
   */
  annotateOrphan?: (info: OrphanInfo) => Promise<void> | void;
}

export interface RetryConfig {
  /** Number of attempts total (incl. the first). Default 3. */
  attempts?: number;
  /** Base delay in ms; doubles each attempt. Default 1000. */
  baseDelayMs?: number;
  /** Sleep injection for tests. */
  sleep?: (ms: number) => Promise<void>;
}

export interface OrphanInfo {
  step: string;
  result: unknown;
  error: Error;
  timestamp: string;
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
  /** Steps whose resources were stamped as orphaned because rollback failed terminally. */
  orphaned: string[];
}

export type CompositeResult = CompositeSuccess | CompositeFailure;

const defaultSleep = (ms: number): Promise<void> =>
  new Promise((resolve) => setTimeout(resolve, ms));

/**
 * Run `fn` with exponential backoff. Resolves with the last result or
 * rejects with the last thrown error.
 */
export async function retryWithBackoff<T>(
  fn: (attempt: number) => Promise<T>,
  cfg: RetryConfig = {}
): Promise<T> {
  const attempts = Math.max(1, cfg.attempts ?? 3);
  const base = cfg.baseDelayMs ?? 1000;
  const sleep = cfg.sleep ?? defaultSleep;
  let lastErr: unknown;
  for (let i = 0; i < attempts; i++) {
    try {
      return await fn(i);
    } catch (err) {
      lastErr = err;
      if (i === attempts - 1) break;
      await sleep(base * 2 ** i);
    }
  }
  throw lastErr instanceof Error ? lastErr : new Error(String(lastErr));
}

export async function runComposite<Ctx>(opts: CompositeOptions<Ctx>): Promise<CompositeResult> {
  const { ctx, steps, logger, rollbackRetry, annotateOrphan } = opts;
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
      const orphaned: string[] = [];
      // rollback in reverse order with exponential backoff
      for (let i = completed.length - 1; i >= 0; i--) {
        const done = completed[i]!;
        const original = steps.find((s) => s.name === done.name);
        if (!original?.rollback) continue;
        try {
          await retryWithBackoff(
            () => Promise.resolve(original.rollback!(ctx, done.result)),
            rollbackRetry
          );
          rolledBack.push(done.name);
          logger?.info({ step: done.name }, 'composite.rollback.success');
        } catch (rbErr) {
          const rollbackError = rbErr instanceof Error ? rbErr : new Error(String(rbErr));
          rollbackErrors.push({ step: done.name, error: rollbackError });
          logger?.error({ step: done.name, err: rollbackError }, 'composite.rollback.failed');
          if (annotateOrphan) {
            try {
              await annotateOrphan({
                step: done.name,
                result: done.result,
                error: rollbackError,
                timestamp: new Date().toISOString(),
              });
              orphaned.push(done.name);
            } catch (annErr) {
              logger?.error({ step: done.name, err: annErr }, 'composite.orphan.annotate_failed');
            }
          }
        }
      }
      return {
        success: false,
        completed,
        failedStep: step.name,
        error,
        rolledBack,
        rollbackErrors,
        orphaned,
      };
    }
  }

  return { success: true, completed };
}

// ---------------------------------------------------------------------------
// Orphan sweeper
// ---------------------------------------------------------------------------

export const ORPHAN_ANNOTATION = 'novanas.io/rollback-orphan';
export const ORPHAN_ABANDONED_ANNOTATION = 'novanas.io/rollback-orphan-abandoned';
export const ORPHAN_ABANDON_AFTER_DAYS = 7;

export interface OrphanCandidate {
  /** Opaque key used to identify the resource across kinds. */
  id: string;
  /** Kind (e.g. 'Dataset', 'Share'). */
  kind: string;
  /** Name of the resource. */
  name: string;
  /** Namespace, if any. */
  namespace?: string;
  /** Value of the `novanas.io/rollback-orphan` annotation (ISO timestamp). */
  annotatedAt: string;
}

export interface OrphanSweeperAdapter {
  /**
   * Return the current list of resources still wearing the orphan annotation.
   */
  list(): Promise<OrphanCandidate[]>;
  /**
   * Re-run the kind-specific rollback. Resolves on success; rejects to keep
   * the candidate in the queue for the next pass.
   */
  cleanup(candidate: OrphanCandidate): Promise<void>;
  /**
   * Swap the candidate's `rollback-orphan` annotation for
   * `rollback-orphan-abandoned` once enough time has elapsed without a
   * successful cleanup.
   */
  markAbandoned(candidate: OrphanCandidate): Promise<void>;
}

export interface OrphanSweeperOptions {
  adapter: OrphanSweeperAdapter;
  logger: FastifyBaseLogger;
  /** How old (in days) an orphan must be before we give up. Default 7. */
  abandonAfterDays?: number;
  /** Re-run cadence. Default 24h. */
  intervalMs?: number;
  /** Run a pass immediately on start. Default true. */
  runOnStart?: boolean;
  /** Clock injection for tests. */
  now?: () => Date;
}

export interface OrphanSweeperHandle {
  runOnce(): Promise<{ cleaned: string[]; abandoned: string[] }>;
  stop(): void;
}

export function startOrphanSweeper(opts: OrphanSweeperOptions): OrphanSweeperHandle {
  const {
    adapter,
    logger,
    abandonAfterDays = ORPHAN_ABANDON_AFTER_DAYS,
    intervalMs = 24 * 3600 * 1000,
    now = () => new Date(),
  } = opts;

  async function runOnce(): Promise<{ cleaned: string[]; abandoned: string[] }> {
    const cleaned: string[] = [];
    const abandoned: string[] = [];
    let candidates: OrphanCandidate[] = [];
    try {
      candidates = await adapter.list();
    } catch (err) {
      logger.error({ err }, 'orphan_sweeper.list_failed');
      return { cleaned, abandoned };
    }

    const cutoff = now().getTime() - abandonAfterDays * 24 * 3600 * 1000;
    for (const c of candidates) {
      const annotatedMs = Date.parse(c.annotatedAt);
      const isStale = Number.isFinite(annotatedMs) && annotatedMs < cutoff;
      try {
        await adapter.cleanup(c);
        cleaned.push(c.id);
        logger.info({ orphan: c.id, kind: c.kind }, 'orphan_sweeper.cleaned');
      } catch (err) {
        if (isStale) {
          try {
            await adapter.markAbandoned(c);
            abandoned.push(c.id);
            logger.warn({ orphan: c.id, kind: c.kind }, 'orphan_sweeper.abandoned');
          } catch (annErr) {
            logger.error({ err: annErr, orphan: c.id }, 'orphan_sweeper.abandon_annotate_failed');
          }
        } else {
          logger.warn({ err, orphan: c.id, kind: c.kind }, 'orphan_sweeper.cleanup_failed');
        }
      }
    }
    return { cleaned, abandoned };
  }

  if (opts.runOnStart !== false) void runOnce();
  const timer = setInterval(() => {
    void runOnce();
  }, intervalMs);
  if (typeof (timer as { unref?: () => void }).unref === 'function') {
    (timer as unknown as { unref: () => void }).unref();
  }

  return {
    runOnce,
    stop: () => clearInterval(timer),
  };
}
