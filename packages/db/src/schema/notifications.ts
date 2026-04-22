import { index, pgEnum, pgTable, text, timestamp, uuid, varchar } from 'drizzle-orm/pg-core';
import { users } from './users.js';

export const notificationSeverity = pgEnum('notification_severity', [
  'info',
  'warning',
  'critical',
]);

export const notifications = pgTable(
  'notifications',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    userId: uuid('user_id')
      .notNull()
      .references(() => users.id, { onDelete: 'cascade' }),
    severity: notificationSeverity('severity').notNull().default('info'),
    title: varchar('title', { length: 255 }).notNull(),
    body: text('body').notNull(),
    link: varchar('link', { length: 1024 }),
    readAt: timestamp('read_at', { withTimezone: true }),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => ({
    userReadIdx: index('notifications_user_read_idx').on(table.userId, table.readAt),
    createdIdx: index('notifications_created_idx').on(table.createdAt),
  })
);

export type Notification = typeof notifications.$inferSelect;
export type NewNotification = typeof notifications.$inferInsert;
