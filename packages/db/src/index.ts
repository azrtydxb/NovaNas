import { migrate as drizzleMigrate } from 'drizzle-orm/postgres-js/migrator';
import type { NovaNasDb } from './client.js';

export * from './schema/index.js';
export * from './client.js';

export interface MigrateOptions {
  /** Directory containing drizzle-generated SQL migrations. */
  migrationsFolder?: string;
}

/**
 * Apply pending migrations. Defaults to the `migrations/` folder shipped in
 * this package. Idempotent: drizzle tracks applied versions in its own
 * bookkeeping table.
 */
export async function migrate(db: NovaNasDb, options: MigrateOptions = {}): Promise<void> {
  await drizzleMigrate(db, {
    migrationsFolder: options.migrationsFolder ?? './migrations',
  });
}
