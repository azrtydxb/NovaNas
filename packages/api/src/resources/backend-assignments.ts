import { type BackendAssignment, BackendAssignmentSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildBackendAssignmentResource(db: DbClient): PgResource<BackendAssignment> {
  return new PgResource<BackendAssignment>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'BackendAssignment',
    schema: BackendAssignmentSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  const resource = buildBackendAssignmentResource(db);
  registerCrudRoutes<BackendAssignment>({
    app,
    basePath: '/api/v1/backend-assignments',
    tag: 'backend-assignments',
    kind: 'BackendAssignment',
    resource,
    schema: BackendAssignmentSchema,
  });
}
