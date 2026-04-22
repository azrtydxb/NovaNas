import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type SmartPolicy, SmartPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildSmartPolicyResource(api: CustomObjectsApi): CrdResource<SmartPolicy> {
  return new CrdResource<SmartPolicy>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'smartpolicies' },
    schema: SmartPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<SmartPolicy>({
    app,
    basePath: '/api/v1/smart-policies',
    tag: 'smart-policies',
    kind: 'SmartPolicy',
    resource: buildSmartPolicyResource(api),
    schema: SmartPolicySchema,
  });
}
