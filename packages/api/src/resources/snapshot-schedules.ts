import { type SnapshotSchedule, SnapshotScheduleSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildSnapshotScheduleResource(db: DbClient): PgResource<SnapshotSchedule> {
  return new PgResource<SnapshotSchedule>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'SnapshotSchedule',
    schema: SnapshotScheduleSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<SnapshotSchedule>({
    app,
    basePath: '/api/v1/snapshot-schedules',
    tag: 'snapshot-schedules',
    kind: 'SnapshotSchedule',
    resource: buildSnapshotScheduleResource(db),
    schema: SnapshotScheduleSchema,
  });
}
