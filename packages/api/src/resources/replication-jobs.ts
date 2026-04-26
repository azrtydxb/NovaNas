import { type ReplicationJob, ReplicationJobSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildReplicationJobResource(db: DbClient): PgResource<ReplicationJob> {
  return new PgResource<ReplicationJob>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ReplicationJob',
    schema: ReplicationJobSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<ReplicationJob>({
    app,
    basePath: '/api/v1/replication-jobs',
    tag: 'replication-jobs',
    kind: 'ReplicationJob',
    resource: buildReplicationJobResource(db),
    schema: ReplicationJobSchema,
  });
}
