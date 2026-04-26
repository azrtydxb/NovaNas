import { type ScrubSchedule, ScrubScheduleSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildScrubScheduleResource(db: DbClient): PgResource<ScrubSchedule> {
  return new PgResource<ScrubSchedule>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ScrubSchedule',
    schema: ScrubScheduleSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<ScrubSchedule>({
    app,
    basePath: '/api/v1/scrub-schedules',
    tag: 'scrub-schedules',
    kind: 'ScrubSchedule',
    resource: buildScrubScheduleResource(db),
    schema: ScrubScheduleSchema,
  });
}
