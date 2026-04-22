import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type AppInstance, AppInstanceSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes, userNamespaceResolver } from './_register.js';

export function buildAppInstanceResource(api: CustomObjectsApi): CrdResource<AppInstance> {
  return new CrdResource<AppInstance>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'appinstances' },
    schema: AppInstanceSchema,
    namespaced: true,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<AppInstance>({
    app,
    basePath: '/api/v1/apps',
    tag: 'apps',
    kind: 'AppInstance',
    resource: buildAppInstanceResource(api),
    schema: AppInstanceSchema,
    resolveNamespace: userNamespaceResolver,
  });
}
