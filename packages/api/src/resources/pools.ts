import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type StoragePool, StoragePoolSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { RegisterValidationError, registerCrudRoutes } from './_register.js';

export function buildPoolResource(api: CustomObjectsApi): CrdResource<StoragePool> {
  return new CrdResource<StoragePool>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'storagepools' },
    schema: StoragePoolSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  const resource = buildPoolResource(api);
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
      const incoming = body as { spec?: { tier?: string } };
      const tier = incoming?.spec?.tier;
      if (!tier) return;
      const existing = await resource.list({});
      for (const pool of existing.items) {
        if (pool.spec?.tier === tier) {
          throw new RegisterValidationError(
            `Tier ${tier} is already used by pool '${pool.metadata.name}'. ` +
              `Each pool must use a distinct performance tier.`
          );
        }
      }
    },
  });
}
