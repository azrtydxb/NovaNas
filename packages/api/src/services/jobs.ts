import { type Job, jobs } from '@novanas/db';
import { and, desc, eq } from 'drizzle-orm';
import type { Redis } from 'ioredis';
import type { DbClient } from './db.js';

/**
 * Jobs service. Backed by the `jobs` table in `@novanas/db` and publishes
 * state transitions to Redis pub/sub on `novanas:events:job:<id>`.
 */

export type JobState = 'queued' | 'running' | 'succeeded' | 'failed' | 'cancelled';

export interface CreateJobInput {
  kind: string;
  params: Record<string, unknown>;
  ownerId?: string | null;
}

export interface UpdateJobInput {
  state?: JobState;
  progressPercent?: number;
  result?: Record<string, unknown>;
  error?: string | null;
  startedAt?: Date;
  finishedAt?: Date;
}

export interface ListJobsFilter {
  state?: JobState;
  kind?: string;
  ownerId?: string;
  limit?: number;
}

const CHANNEL_PREFIX = 'novanas:events:';

export class JobsService {
  constructor(
    private readonly db: DbClient,
    private readonly redis: Redis | null
  ) {}

  async create(input: CreateJobInput): Promise<Job> {
    const [row] = await this.db
      .insert(jobs)
      .values({
        kind: input.kind,
        params: input.params,
        ownerId: input.ownerId ?? null,
        state: 'queued',
      })
      .returning();
    await this.publish(row!, 'created');
    return row!;
  }

  async get(id: string): Promise<Job | null> {
    const rows = await this.db.select().from(jobs).where(eq(jobs.id, id)).limit(1);
    return rows[0] ?? null;
  }

  async list(filter: ListJobsFilter): Promise<Job[]> {
    const conditions = [];
    if (filter.state) conditions.push(eq(jobs.state, filter.state));
    if (filter.kind) conditions.push(eq(jobs.kind, filter.kind));
    if (filter.ownerId) conditions.push(eq(jobs.ownerId, filter.ownerId));
    const where = conditions.length > 0 ? and(...conditions) : undefined;
    const q = this.db.select().from(jobs);
    const withWhere = where ? q.where(where) : q;
    return withWhere.orderBy(desc(jobs.createdAt)).limit(Math.min(filter.limit ?? 50, 500));
  }

  async update(id: string, patch: UpdateJobInput): Promise<Job | null> {
    const values: Record<string, unknown> = { updatedAt: new Date() };
    if (patch.state !== undefined) values.state = patch.state;
    if (patch.progressPercent !== undefined) values.progressPercent = patch.progressPercent;
    if (patch.result !== undefined) values.result = patch.result;
    if (patch.error !== undefined) values.error = patch.error;
    if (patch.startedAt !== undefined) values.startedAt = patch.startedAt;
    if (patch.finishedAt !== undefined) values.finishedAt = patch.finishedAt;
    const [row] = await this.db.update(jobs).set(values).where(eq(jobs.id, id)).returning();
    if (!row) return null;
    await this.publish(row, 'updated');
    return row;
  }

  async cancel(id: string): Promise<Job | null> {
    const existing = await this.get(id);
    if (!existing) return null;
    if (existing.state === 'succeeded' || existing.state === 'failed') {
      return existing; // terminal
    }
    const [row] = await this.db
      .update(jobs)
      .set({ state: 'cancelled', finishedAt: new Date(), updatedAt: new Date() })
      .where(eq(jobs.id, id))
      .returning();
    if (row) await this.publish(row, 'cancelled');
    return row ?? null;
  }

  private async publish(row: Job, event: string): Promise<void> {
    if (!this.redis) return;
    try {
      await this.redis.publish(
        `${CHANNEL_PREFIX}job:${row.id}`,
        JSON.stringify({ event, payload: row })
      );
    } catch {
      /* best effort */
    }
  }
}
