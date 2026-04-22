import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Vlan, VlanSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildVlanResource(api: CustomObjectsApi): CrdResource<Vlan> {
  return new CrdResource<Vlan>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'vlans' },
    schema: VlanSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Vlan>({
    app,
    basePath: '/api/v1/vlans',
    tag: 'vlans',
    kind: 'Vlan',
    resource: buildVlanResource(api),
    schema: VlanSchema,
  });
}
