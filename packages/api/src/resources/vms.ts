import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Vm, VmSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes, userNamespaceResolver } from './_register.js';

export function buildVmResource(api: CustomObjectsApi): CrdResource<Vm> {
  return new CrdResource<Vm>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'vms' },
    schema: VmSchema,
    namespaced: true,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Vm>({
    app,
    basePath: '/api/v1/vms',
    tag: 'vms',
    kind: 'Vm',
    resource: buildVmResource(api),
    schema: VmSchema,
    resolveNamespace: userNamespaceResolver,
  });
}
