import type { NovaNasDb } from '@novanas/db';
import { createDb } from '@novanas/db';
import type { Env } from '../env.js';

/**
 * The API-side handle on the typed Drizzle client from `@novanas/db`.
 *
 * We export the `NovaNasDb` type directly so callers can pass the client
 * through to services. In tests, `db` is `null` and services silently
 * skip DB work.
 */
export type DbClient = NovaNasDb;

export async function createDbClient(env: Env): Promise<DbClient> {
  return createDb(env.DATABASE_URL);
}
