import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type UpdatePolicy, UpdatePolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildUpdatePolicyResource(api: CustomObjectsApi): CrdResource<UpdatePolicy> {
  return new CrdResource<UpdatePolicy>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'updatepolicies' },
    schema: UpdatePolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerSingletonRoutes<UpdatePolicy>({
    app,
    basePath: '/api/v1/update-policy',
    tag: 'update-policy',
    kind: 'UpdatePolicy',
    resource: buildUpdatePolicyResource(api),
    schema: UpdatePolicySchema,
  });
}
