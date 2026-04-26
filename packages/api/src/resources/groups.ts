import { type Group, GroupSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildGroupResource(db: DbClient): PgResource<Group> {
  return new PgResource<Group>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Group',
    schema: GroupSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Group>({
    app,
    basePath: '/api/v1/groups',
    tag: 'groups',
    kind: 'Group',
    resource: buildGroupResource(db),
    schema: GroupSchema,
  });
}
