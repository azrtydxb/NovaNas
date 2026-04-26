import { type IsoLibrary, IsoLibrarySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildIsoLibraryResource(db: DbClient): PgResource<IsoLibrary> {
  return new PgResource<IsoLibrary>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'IsoLibrary',
    schema: IsoLibrarySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<IsoLibrary>({
    app,
    basePath: '/api/v1/iso-libraries',
    tag: 'iso-libraries',
    kind: 'IsoLibrary',
    resource: buildIsoLibraryResource(db),
    schema: IsoLibrarySchema,
  });
}
