import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type ServiceLevelObjective, ServiceLevelObjectiveSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildServiceLevelObjectiveResource(
  api: CustomObjectsApi
): CrdResource<ServiceLevelObjective> {
  return new CrdResource<ServiceLevelObjective>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'servicelevelobjectives' },
    schema: ServiceLevelObjectiveSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<ServiceLevelObjective>({
    app,
    basePath: '/api/v1/slos',
    tag: 'slos',
    kind: 'ServiceLevelObjective',
    resource: buildServiceLevelObjectiveResource(api),
    schema: ServiceLevelObjectiveSchema,
  });
}
