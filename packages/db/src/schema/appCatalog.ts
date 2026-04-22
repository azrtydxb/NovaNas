import {
  customType,
  index,
  jsonb,
  pgTable,
  timestamp,
  uniqueIndex,
  uuid,
  varchar,
} from 'drizzle-orm/pg-core';

const bytea = customType<{ data: Buffer; default: false }>({
  dataType() {
    return 'bytea';
  },
});

export interface AppCatalogMetadata {
  description?: string;
  categories?: string[];
  maintainers?: Array<{ name: string; email?: string; url?: string }>;
  home?: string;
  sources?: string[];
  appVersion?: string;
  keywords?: string[];
  [key: string]: unknown;
}

/**
 * Cached entries from remote Helm / app catalogs. Keeps the UI snappy and
 * lets us operate when the upstream catalog is momentarily unavailable.
 */
export const appCatalogCache = pgTable(
  'app_catalog_cache',
  {
    id: uuid('id').primaryKey().defaultRandom(),
    catalogName: varchar('catalog_name', { length: 255 }).notNull(),
    appName: varchar('app_name', { length: 255 }).notNull(),
    version: varchar('version', { length: 64 }).notNull(),
    metadata: jsonb('metadata').notNull().$type<AppCatalogMetadata>().default({}),
    icon: bytea('icon'),
    fetchedAt: timestamp('fetched_at', { withTimezone: true }).notNull().defaultNow(),
    createdAt: timestamp('created_at', { withTimezone: true }).notNull().defaultNow(),
  },
  (table) => ({
    catalogAppVersionIdx: uniqueIndex('app_catalog_cache_catalog_app_version_idx').on(
      table.catalogName,
      table.appName,
      table.version
    ),
    appIdx: index('app_catalog_cache_app_idx').on(table.appName),
    fetchedIdx: index('app_catalog_cache_fetched_idx').on(table.fetchedAt),
  })
);

export type AppCatalogEntry = typeof appCatalogCache.$inferSelect;
export type NewAppCatalogEntry = typeof appCatalogCache.$inferInsert;
