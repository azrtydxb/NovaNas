import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type SnapshotSchedule, SnapshotScheduleSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildSnapshotScheduleResource(
  api: CustomObjectsApi
): CrdResource<SnapshotSchedule> {
  return new CrdResource<SnapshotSchedule>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'snapshotschedules' },
    schema: SnapshotScheduleSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<SnapshotSchedule>({
    app,
    basePath: '/api/v1/snapshot-schedules',
    tag: 'snapshot-schedules',
    kind: 'SnapshotSchedule',
    resource: buildSnapshotScheduleResource(api),
    schema: SnapshotScheduleSchema,
  });
}
