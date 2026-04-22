import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Ingress, IngressSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildIngressResource(api: CustomObjectsApi): CrdResource<Ingress> {
  return new CrdResource<Ingress>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'ingresses' },
    schema: IngressSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Ingress>({
    app,
    basePath: '/api/v1/ingresses',
    tag: 'ingresses',
    kind: 'Ingress',
    resource: buildIngressResource(api),
    schema: IngressSchema,
  });
}
