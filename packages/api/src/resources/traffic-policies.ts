import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type TrafficPolicy, TrafficPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildTrafficPolicyResource(api: CustomObjectsApi): CrdResource<TrafficPolicy> {
  return new CrdResource<TrafficPolicy>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'trafficpolicies' },
    schema: TrafficPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<TrafficPolicy>({
    app,
    basePath: '/api/v1/traffic-policies',
    tag: 'traffic-policies',
    kind: 'TrafficPolicy',
    resource: buildTrafficPolicyResource(api),
    schema: TrafficPolicySchema,
  });
}
