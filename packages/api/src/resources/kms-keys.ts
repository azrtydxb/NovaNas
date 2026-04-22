import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type KmsKey, KmsKeySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildKmsKeyResource(api: CustomObjectsApi): CrdResource<KmsKey> {
  return new CrdResource<KmsKey>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'kmskeys' },
    schema: KmsKeySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<KmsKey>({
    app,
    basePath: '/api/v1/kms-keys',
    tag: 'kms-keys',
    kind: 'KmsKey',
    resource: buildKmsKeyResource(api),
    schema: KmsKeySchema,
  });
}
