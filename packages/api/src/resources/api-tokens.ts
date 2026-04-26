import { type ApiToken, ApiTokenSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildApiTokenResource(db: DbClient): PgResource<ApiToken> {
  return new PgResource<ApiToken>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ApiToken',
    schema: ApiTokenSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<ApiToken>({
    app,
    basePath: '/api/v1/api-tokens',
    tag: 'api-tokens',
    kind: 'ApiToken',
    resource: buildApiTokenResource(db),
    schema: ApiTokenSchema,
  });
}
