import { type Share, ShareSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildShareResource(db: DbClient): PgResource<Share> {
  return new PgResource<Share>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Share',
    schema: ShareSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Share>({
    app,
    basePath: '/api/v1/shares',
    tag: 'shares',
    kind: 'Share',
    resource: buildShareResource(db),
    schema: ShareSchema,
  });
}
