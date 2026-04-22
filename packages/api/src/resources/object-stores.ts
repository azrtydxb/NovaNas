import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type ObjectStore, ObjectStoreSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildObjectStoreResource(api: CustomObjectsApi): CrdResource<ObjectStore> {
  return new CrdResource<ObjectStore>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'objectstores' },
    schema: ObjectStoreSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<ObjectStore>({
    app,
    basePath: '/api/v1/object-stores',
    tag: 'object-stores',
    kind: 'ObjectStore',
    resource: buildObjectStoreResource(api),
    schema: ObjectStoreSchema,
  });
}
