import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Share, ShareSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildShareResource(api: CustomObjectsApi): CrdResource<Share> {
  return new CrdResource<Share>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'shares' },
    schema: ShareSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Share>({
    app,
    basePath: '/api/v1/shares',
    tag: 'shares',
    kind: 'Share',
    resource: buildShareResource(api),
    schema: ShareSchema,
  });
}
