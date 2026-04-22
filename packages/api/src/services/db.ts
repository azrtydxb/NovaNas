import type { Env } from '../env.js';

/**
 * Drizzle client factory. The real typed client lives in `@novanas/db`;
 * this file is the API-side adapter. Wave 2 coordinator wires the actual
 * export once `@novanas/db` exposes `createDb(connectionString)`.
 */
export interface DbClient {
  /** Placeholder — real shape is the Drizzle client from @novanas/db. */
  readonly url: string;
}

export async function createDbClient(env: Env): Promise<DbClient> {
  // TODO(wave-3): swap for `import { createDb } from '@novanas/db'` once
  // A2-DB exports a factory. Keeping as a narrow stub so the app boots
  // during scaffold.
  return { url: env.DATABASE_URL };
}
