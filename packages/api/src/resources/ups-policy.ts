import { type UpsPolicy, UpsPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildUpsPolicyResource(db: DbClient): PgResource<UpsPolicy> {
  return new PgResource<UpsPolicy>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'UpsPolicy',
    schema: UpsPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<UpsPolicy>({
    app,
    basePath: '/api/v1/ups-policy',
    tag: 'ups-policy',
    kind: 'UpsPolicy',
    resource: buildUpsPolicyResource(db),
    schema: UpsPolicySchema,
  });
}
