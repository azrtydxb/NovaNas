import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type EncryptionPolicy, EncryptionPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildEncryptionPolicyResource(
  api: CustomObjectsApi
): CrdResource<EncryptionPolicy> {
  return new CrdResource<EncryptionPolicy>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'encryptionpolicies' },
    schema: EncryptionPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<EncryptionPolicy>({
    app,
    basePath: '/api/v1/encryption-policies',
    tag: 'encryption-policies',
    kind: 'EncryptionPolicy',
    resource: buildEncryptionPolicyResource(api),
    schema: EncryptionPolicySchema,
  });
}
