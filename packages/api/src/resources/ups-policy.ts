import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type UpsPolicy, UpsPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildUpsPolicyResource(api: CustomObjectsApi): CrdResource<UpsPolicy> {
  return new CrdResource<UpsPolicy>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'upspolicies' },
    schema: UpsPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerSingletonRoutes<UpsPolicy>({
    app,
    basePath: '/api/v1/ups-policy',
    tag: 'ups-policy',
    kind: 'UpsPolicy',
    resource: buildUpsPolicyResource(api),
    schema: UpsPolicySchema,
  });
}
