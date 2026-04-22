import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Bond, BondSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildBondResource(api: CustomObjectsApi): CrdResource<Bond> {
  return new CrdResource<Bond>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'bonds' },
    schema: BondSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Bond>({
    app,
    basePath: '/api/v1/bonds',
    tag: 'bonds',
    kind: 'Bond',
    resource: buildBondResource(api),
    schema: BondSchema,
  });
}
