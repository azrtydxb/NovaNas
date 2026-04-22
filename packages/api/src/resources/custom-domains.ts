import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type CustomDomain, CustomDomainSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildCustomDomainResource(api: CustomObjectsApi): CrdResource<CustomDomain> {
  return new CrdResource<CustomDomain>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'customdomains' },
    schema: CustomDomainSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<CustomDomain>({
    app,
    basePath: '/api/v1/custom-domains',
    tag: 'custom-domains',
    kind: 'CustomDomain',
    resource: buildCustomDomainResource(api),
    schema: CustomDomainSchema,
  });
}
