import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerAppCatalogs } from '../resources/app-catalogs.js';
import { registerUnavailable } from './_unavailable.js';

export async function appCatalogRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerAppCatalogs(app, api);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/app-catalogs',
      summary: 'List app catalogs',
      tag: 'app-catalogs',
    },
    {
      method: 'POST',
      url: '/api/v1/app-catalogs',
      summary: 'Create an app catalog',
      tag: 'app-catalogs',
    },
    {
      method: 'GET',
      url: '/api/v1/app-catalogs/:name',
      summary: 'Get an app catalog',
      tag: 'app-catalogs',
    },
    {
      method: 'PATCH',
      url: '/api/v1/app-catalogs/:name',
      summary: 'Update an app catalog',
      tag: 'app-catalogs',
    },
    {
      method: 'DELETE',
      url: '/api/v1/app-catalogs/:name',
      summary: 'Delete an app catalog',
      tag: 'app-catalogs',
    },
  ]);
}
