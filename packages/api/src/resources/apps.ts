import { type AppInstance, AppInstanceSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes, userNamespaceResolver } from './_register.js';

export function buildAppInstanceResource(db: DbClient): PgResource<AppInstance> {
  return new PgResource<AppInstance>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'AppInstance',
    schema: AppInstanceSchema,
    namespaced: true,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<AppInstance>({
    app,
    basePath: '/api/v1/apps',
    tag: 'apps',
    kind: 'AppInstance',
    resource: buildAppInstanceResource(db),
    schema: AppInstanceSchema,
    resolveNamespace: userNamespaceResolver,
  });
}
