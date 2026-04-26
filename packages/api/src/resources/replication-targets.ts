import { type ReplicationTarget, ReplicationTargetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildReplicationTargetResource(db: DbClient): PgResource<ReplicationTarget> {
  return new PgResource<ReplicationTarget>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ReplicationTarget',
    schema: ReplicationTargetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<ReplicationTarget>({
    app,
    basePath: '/api/v1/replication-targets',
    tag: 'replication-targets',
    kind: 'ReplicationTarget',
    resource: buildReplicationTargetResource(db),
    schema: ReplicationTargetSchema,
  });
}
