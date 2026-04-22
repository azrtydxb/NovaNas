import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type AppCatalog, AppCatalogSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildAppCatalogResource(api: CustomObjectsApi): CrdResource<AppCatalog> {
  return new CrdResource<AppCatalog>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'appcatalogs' },
    schema: AppCatalogSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<AppCatalog>({
    app,
    basePath: '/api/v1/app-catalogs',
    tag: 'app-catalogs',
    kind: 'AppCatalog',
    resource: buildAppCatalogResource(api),
    schema: AppCatalogSchema,
  });
}
