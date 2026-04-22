import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type StoragePool, StoragePoolSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildPoolResource(api: CustomObjectsApi): CrdResource<StoragePool> {
  return new CrdResource<StoragePool>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'storagepools' },
    schema: StoragePoolSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<StoragePool>({
    app,
    basePath: '/api/v1/pools',
    tag: 'pools',
    kind: 'StoragePool',
    resource: buildPoolResource(api),
    schema: StoragePoolSchema,
  });
}
