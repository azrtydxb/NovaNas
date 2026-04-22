import { type PostgresJsDatabase, drizzle } from 'drizzle-orm/postgres-js';
import postgres from 'postgres';
import * as schema from './schema/index.js';

export type NovaNasSchema = typeof schema;

/**
 * Typed Drizzle client with the full NovaNas schema registered. Consumers
 * (`@novanas/api`, operators, CLI) should take this as a constructor param
 * rather than call `createDb` themselves, so tests can inject a fake.
 */
export type NovaNasDb = PostgresJsDatabase<NovaNasSchema>;

export interface CreateDbOptions {
  /** Maximum pool size. Defaults to 10. */
  max?: number;
  /** Enable `ssl` mode. Defaults to undefined (driver default). */
  ssl?: boolean | 'require' | 'allow' | 'prefer' | 'verify-full';
}

/**
 * Build a typed Drizzle client backed by postgres-js. The returned handle
 * owns the underlying pool; hold it for the lifetime of the process.
 */
export function createDb(url: string, options: CreateDbOptions = {}): NovaNasDb {
  const client = postgres(url, {
    max: options.max ?? 10,
    ssl: options.ssl,
  });
  return drizzle(client, { schema });
}
