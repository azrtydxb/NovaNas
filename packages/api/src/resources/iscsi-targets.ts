import { type IscsiTarget, IscsiTargetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildIscsiTargetResource(db: DbClient): PgResource<IscsiTarget> {
  return new PgResource<IscsiTarget>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'IscsiTarget',
    schema: IscsiTargetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<IscsiTarget>({
    app,
    basePath: '/api/v1/iscsi-targets',
    tag: 'iscsi-targets',
    kind: 'IscsiTarget',
    resource: buildIscsiTargetResource(db),
    schema: IscsiTargetSchema,
  });
}
