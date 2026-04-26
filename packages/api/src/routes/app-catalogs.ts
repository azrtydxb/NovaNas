import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/app-catalogs.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function appCatalogRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/app-catalogs', summary: 'List AppCatalogs', tag: 'app-catalogs' },
    { method: 'POST', url: '/api/v1/app-catalogs', summary: 'Create a AppCatalog', tag: 'app-catalogs' },
    { method: 'GET', url: '/api/v1/app-catalogs/:name', summary: 'Get a AppCatalog', tag: 'app-catalogs' },
    { method: 'PATCH', url: '/api/v1/app-catalogs/:name', summary: 'Update a AppCatalog', tag: 'app-catalogs' },
    { method: 'DELETE', url: '/api/v1/app-catalogs/:name', summary: 'Delete a AppCatalog', tag: 'app-catalogs' },
  ]);
}
