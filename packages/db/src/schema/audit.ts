import { index, jsonb, pgEnum, pgTable, timestamp, uuid, varchar } from 'drizzle-orm/pg-core';
import { users } from './users.js';

export const auditActorType = pgEnum('audit_actor_type', [
  'user',
  'system',
  'operator',
  'apiToken',
]);

export const auditOutcome = pgEnum('audit_outcome', ['success', 'failure', 'denied']);

/**
 * Append-only audit log. Every mutating API call, operator reconcile, and
 * security event produces one row. See docs/12-observability.md.
 */
export const auditLog = pgTable(
  'audit_log',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    timestamp: timestamp('timestamp', { withTimezone: true }).notNull().defaultNow(),
    actorId: uuid('actor_id').references(() => users.id, { onDelete: 'set null' }),
    actorType: auditActorType('actor_type').notNull(),
    action: varchar('action', { length: 128 }).notNull(),
    resourceKind: varchar('resource_kind', { length: 128 }).notNull(),
    resourceName: varchar('resource_name', { length: 253 }),
    resourceNamespace: varchar('resource_namespace', { length: 253 }),
    payload: jsonb('payload').$type<Record<string, unknown>>(),
    outcome: auditOutcome('outcome').notNull(),
    sourceIp: varchar('source_ip', { length: 64 }),
    details: jsonb('details').$type<Record<string, unknown>>(),
  },
  (table) => ({
    timestampActorIdx: index('audit_log_timestamp_actor_idx').on(table.timestamp, table.actorId),
    resourceIdx: index('audit_log_resource_idx').on(table.resourceKind, table.resourceName),
    timestampIdx: index('audit_log_timestamp_idx').on(table.timestamp),
    actionIdx: index('audit_log_action_idx').on(table.action),
  })
);

export type AuditLogEntry = typeof auditLog.$inferSelect;
export type NewAuditLogEntry = typeof auditLog.$inferInsert;
