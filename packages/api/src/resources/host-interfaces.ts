import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type HostInterface, HostInterfaceSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildHostInterfaceResource(api: CustomObjectsApi): CrdResource<HostInterface> {
  return new CrdResource<HostInterface>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'hostinterfaces' },
    schema: HostInterfaceSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<HostInterface>({
    app,
    basePath: '/api/v1/host-interfaces',
    tag: 'host-interfaces',
    kind: 'HostInterface',
    resource: buildHostInterfaceResource(api),
    schema: HostInterfaceSchema,
  });
}
