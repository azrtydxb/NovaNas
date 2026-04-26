import { type StoragePool, StoragePoolSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { RegisterValidationError, registerCrudRoutes } from './_register.js';

export function buildPoolResource(db: DbClient): PgResource<StoragePool> {
  return new PgResource<StoragePool>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'StoragePool',
    schema: StoragePoolSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  const resource = buildPoolResource(db);
  registerCrudRoutes<StoragePool>({
    app,
    basePath: '/api/v1/pools',
    tag: 'pools',
    kind: 'StoragePool',
    resource,
    schema: StoragePoolSchema,
    validate: async (action, body) => {
      if (action !== 'create') return;
      // Each performance tier is exclusive across pools — there's no
      // useful semantic for two pools at the same level. Reject early
      // with a 422 the SPA can show inline. The dropdown already
      // disables used tiers in the UI; this guard catches CLI/API
      // callers and races between two simultaneous create attempts.
      // Postgres-backed: this validation now actually holds, because
      // there is no kubectl backdoor to the resource.
      const incoming = body as { spec?: { tier?: string } };
      const tier = incoming?.spec?.tier;
      if (!tier) return;
      const existing = await resource.list({});
      for (const pool of existing.items) {
        if (pool.spec?.tier === tier) {
          throw new RegisterValidationError(
            `Tier ${tier} is already used by pool '${pool.metadata.name}'. Each pool must use a distinct performance tier.`
          );
        }
      }
    },
  });
}
