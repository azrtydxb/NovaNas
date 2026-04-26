import { type ObjectStore, ObjectStoreSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildObjectStoreResource(db: DbClient): PgResource<ObjectStore> {
  return new PgResource<ObjectStore>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ObjectStore',
    schema: ObjectStoreSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<ObjectStore>({
    app,
    basePath: '/api/v1/object-stores',
    tag: 'object-stores',
    kind: 'ObjectStore',
    resource: buildObjectStoreResource(db),
    schema: ObjectStoreSchema,
  });
}
