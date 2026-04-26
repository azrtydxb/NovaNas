import { type AppCatalog, AppCatalogSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildAppCatalogResource(db: DbClient): PgResource<AppCatalog> {
  return new PgResource<AppCatalog>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'AppCatalog',
    schema: AppCatalogSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<AppCatalog>({
    app,
    basePath: '/api/v1/app-catalogs',
    tag: 'app-catalogs',
    kind: 'AppCatalog',
    resource: buildAppCatalogResource(db),
    schema: AppCatalogSchema,
  });
}
