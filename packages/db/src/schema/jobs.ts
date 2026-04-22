import {
  index,
  integer,
  jsonb,
  pgEnum,
  pgTable,
  text,
  timestamp,
  uuid,
  varchar,
} from 'drizzle-orm/pg-core';
import { users } from './users.js';

export const jobState = pgEnum('job_state', [
  'queued',
  'running',
  'succeeded',
  'failed',
  'cancelled',
]);

/**
 * Long-running background jobs (backups, scrubs, imports, etc.) surfaced in
 * the UI. The operators dispatch these; the API server tails them.
 */
export const jobs = pgTable(
  'jobs',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    kind: varchar('kind', { length: 128 }).notNull(),
    state: jobState('state').notNull().default('queued'),
    progressPercent: integer('progress_percent').notNull().default(0),
    startedAt: timestamp('started_at', { withTimezone: true }),
    finishedAt: timestamp('finished_at', { withTimezone: true }),
    params: jsonb('params').notNull().$type<Record<string, unknown>>().default({}),
    result: jsonb('result').$type<Record<string, unknown>>(),
    error: text('error'),
    ownerId: uuid('owner_id').references(() => users.id, { onDelete: 'set null' }),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
    updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => ({
    stateKindIdx: index('jobs_state_kind_idx').on(table.state, table.kind),
    ownerIdx: index('jobs_owner_idx').on(table.ownerId),
    createdIdx: index('jobs_created_idx').on(table.createdAt),
  })
);

export type Job = typeof jobs.$inferSelect;
export type NewJob = typeof jobs.$inferInsert;
