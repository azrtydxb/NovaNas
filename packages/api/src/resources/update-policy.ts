import { type UpdatePolicy, UpdatePolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildUpdatePolicyResource(db: DbClient): PgResource<UpdatePolicy> {
  return new PgResource<UpdatePolicy>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'UpdatePolicy',
    schema: UpdatePolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<UpdatePolicy>({
    app,
    basePath: '/api/v1/update-policy',
    tag: 'update-policy',
    kind: 'UpdatePolicy',
    resource: buildUpdatePolicyResource(db),
    schema: UpdatePolicySchema,
  });
}
