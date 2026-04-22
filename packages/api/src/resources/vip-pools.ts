import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type VipPool, VipPoolSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildVipPoolResource(api: CustomObjectsApi): CrdResource<VipPool> {
  return new CrdResource<VipPool>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'vippools' },
    schema: VipPoolSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<VipPool>({
    app,
    basePath: '/api/v1/vip-pools',
    tag: 'vip-pools',
    kind: 'VipPool',
    resource: buildVipPoolResource(api),
    schema: VipPoolSchema,
  });
}
