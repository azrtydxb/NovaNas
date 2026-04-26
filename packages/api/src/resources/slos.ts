import { type ServiceLevelObjective, ServiceLevelObjectiveSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildServiceLevelObjectiveResource(
  db: DbClient
): PgResource<ServiceLevelObjective> {
  return new PgResource<ServiceLevelObjective>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ServiceLevelObjective',
    schema: ServiceLevelObjectiveSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<ServiceLevelObjective>({
    app,
    basePath: '/api/v1/slos',
    tag: 'slos',
    kind: 'ServiceLevelObjective',
    resource: buildServiceLevelObjectiveResource(db),
    schema: ServiceLevelObjectiveSchema,
  });
}
