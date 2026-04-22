import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type ServicePolicy, ServicePolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildServicePolicyResource(api: CustomObjectsApi): CrdResource<ServicePolicy> {
  return new CrdResource<ServicePolicy>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'servicepolicies' },
    schema: ServicePolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerSingletonRoutes<ServicePolicy>({
    app,
    basePath: '/api/v1/service-policy',
    tag: 'service-policy',
    kind: 'ServicePolicy',
    resource: buildServicePolicyResource(api),
    schema: ServicePolicySchema,
  });
}
