import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type NvmeofTarget, NvmeofTargetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildNvmeofTargetResource(api: CustomObjectsApi): CrdResource<NvmeofTarget> {
  return new CrdResource<NvmeofTarget>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'nvmeoftargets' },
    schema: NvmeofTargetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<NvmeofTarget>({
    app,
    basePath: '/api/v1/nvmeof-targets',
    tag: 'nvmeof-targets',
    kind: 'NvmeofTarget',
    resource: buildNvmeofTargetResource(api),
    schema: NvmeofTargetSchema,
  });
}
