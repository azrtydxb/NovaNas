import { type NfsServer, NfsServerSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildNfsServerResource(db: DbClient): PgResource<NfsServer> {
  return new PgResource<NfsServer>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'NfsServer',
    schema: NfsServerSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<NfsServer>({
    app,
    basePath: '/api/v1/nfs-servers',
    tag: 'nfs-servers',
    kind: 'NfsServer',
    resource: buildNfsServerResource(db),
    schema: NfsServerSchema,
  });
}
