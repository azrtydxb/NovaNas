import { jsonb, pgTable, timestamp, uuid } from 'drizzle-orm/pg-core';
import { users } from './users.js';

/**
 * Opaque per-user preferences blob: theme, collapsed sidebar sections,
 * dashboard layout, favorites, etc. Schema is defined client-side; we
 * only store/retrieve.
 */
export interface UserPreferencesBlob {
  theme?: 'light' | 'dark' | 'system';
  sidebar?: { collapsedSections?: string[] };
  dashboard?: { layout?: Array<{ id: string; x: number; y: number; w: number; h: number }> };
  favorites?: string[];
  [key: string]: unknown;
}

export const userPreferences = pgTable('user_preferences', {
  userId: uuid('user_id')
    .primaryKey()
    .references(() => users.id, { onDelete: 'cascade' }),
  preferences: jsonb('preferences').notNull().$type<UserPreferencesBlob>().default({}),
  updatedAt: timestamp('updated_at', { withTimezone: true }).notNull().defaultNow(),
});

export type UserPreferences = typeof userPreferences.$inferSelect;
export type NewUserPreferences = typeof userPreferences.$inferInsert;
