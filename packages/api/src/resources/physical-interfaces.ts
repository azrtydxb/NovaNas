import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type PhysicalInterface, PhysicalInterfaceSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerReadOnlyRoutes } from './_register_extras.js';

export function buildPhysicalInterfaceResource(
  api: CustomObjectsApi
): CrdResource<PhysicalInterface> {
  return new CrdResource<PhysicalInterface>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'physicalinterfaces' },
    schema: PhysicalInterfaceSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerReadOnlyRoutes<PhysicalInterface>({
    app,
    basePath: '/api/v1/physical-interfaces',
    tag: 'physical-interfaces',
    kind: 'PhysicalInterface',
    resource: buildPhysicalInterfaceResource(api),
    schema: PhysicalInterfaceSchema,
  });
}
