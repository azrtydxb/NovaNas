import { type Dataset, DatasetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildDatasetResource(db: DbClient): PgResource<Dataset> {
  return new PgResource<Dataset>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Dataset',
    schema: DatasetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Dataset>({
    app,
    basePath: '/api/v1/datasets',
    tag: 'datasets',
    kind: 'Dataset',
    resource: buildDatasetResource(db),
    schema: DatasetSchema,
  });
}
