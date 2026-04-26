import { type Snapshot, SnapshotSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildSnapshotResource(db: DbClient): PgResource<Snapshot> {
  return new PgResource<Snapshot>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Snapshot',
    schema: SnapshotSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Snapshot>({
    app,
    basePath: '/api/v1/snapshots',
    tag: 'snapshots',
    kind: 'Snapshot',
    resource: buildSnapshotResource(db),
    schema: SnapshotSchema,
  });
}
