import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type ScrubSchedule, ScrubScheduleSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildScrubScheduleResource(api: CustomObjectsApi): CrdResource<ScrubSchedule> {
  return new CrdResource<ScrubSchedule>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'scrubschedules' },
    schema: ScrubScheduleSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<ScrubSchedule>({
    app,
    basePath: '/api/v1/scrub-schedules',
    tag: 'scrub-schedules',
    kind: 'ScrubSchedule',
    resource: buildScrubScheduleResource(api),
    schema: ScrubScheduleSchema,
  });
}
