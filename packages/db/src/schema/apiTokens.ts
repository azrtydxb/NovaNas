import {
  index,
  jsonb,
  pgTable,
  timestamp,
  uniqueIndex,
  uuid,
  varchar,
} from 'drizzle-orm/pg-core';
import { users } from './users.js';

/**
 * Long-lived API tokens (e.g. for CLI, CI, scripts). Stored as a salted
 * hash; the raw token is shown exactly once at creation time. Scopes are
 * an array of capability strings (e.g. "pools:read", "apps:write").
 */
export const apiTokens = pgTable(
  'api_tokens',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    userId: uuid('user_id')
      .notNull()
      .references(() => users.id, { onDelete: 'cascade' }),
    name: varchar('name', { length: 255 }).notNull(),
    tokenHash: varchar('token_hash', { length: 128 }).notNull(),
    scopes: jsonb('scopes').notNull().$type<string[]>().default([]),
    expiresAt: timestamp('expires_at', { withTimezone: true }),
    lastUsedAt: timestamp('last_used_at', { withTimezone: true }),
    revokedAt: timestamp('revoked_at', { withTimezone: true }),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => ({
    tokenHashIdx: uniqueIndex('api_tokens_token_hash_idx').on(table.tokenHash),
    userIdx: index('api_tokens_user_idx').on(table.userId),
    expiresIdx: index('api_tokens_expires_idx').on(table.expiresAt),
  }),
);

export type ApiToken = typeof apiTokens.$inferSelect;
export type NewApiToken = typeof apiTokens.$inferInsert;
