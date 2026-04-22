import {
  index,
  jsonb,
  pgTable,
  timestamp,
  uuid,
  varchar,
} from 'drizzle-orm/pg-core';

export interface MetricAggregation {
  min?: number;
  max?: number;
  avg?: number;
  sum?: number;
  count?: number;
  p50?: number;
  p95?: number;
  p99?: number;
  [key: string]: number | undefined;
}

/**
 * Downsampled metric windows. Prometheus remains the source for live queries;
 * these rollups back the dashboard so initial paint is cheap and doesn't
 * hammer the TSDB on every page load.
 */
export const metricRollups = pgTable(
  'metric_rollups',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    metricName: varchar('metric_name', { length: 255 }).notNull(),
    resourceKind: varchar('resource_kind', { length: 128 }).notNull(),
    resourceName: varchar('resource_name', { length: 253 }).notNull(),
    windowStart: timestamp('window_start', { withTimezone: true }).notNull(),
    windowEnd: timestamp('window_end', { withTimezone: true }).notNull(),
    aggregation: jsonb('aggregation').notNull().$type<MetricAggregation>(),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => ({
    metricResourceWindowIdx: index('metric_rollups_metric_resource_window_idx').on(
      table.metricName,
      table.resourceKind,
      table.resourceName,
      table.windowStart,
    ),
    windowIdx: index('metric_rollups_window_idx').on(table.windowStart, table.windowEnd),
  }),
);

export type MetricRollup = typeof metricRollups.$inferSelect;
export type NewMetricRollup = typeof metricRollups.$inferInsert;
