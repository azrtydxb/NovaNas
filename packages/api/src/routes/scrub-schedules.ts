import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/scrub-schedules.js';
import { registerStubs } from './_stubs.js';

export async function scrubSchedulesRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerStubs(app, [
    {
      method: 'GET',
      url: '/api/v1/scrub-schedules',
      summary: 'List scrub schedules',
      tag: 'scrub-schedules',
    },
    {
      method: 'POST',
      url: '/api/v1/scrub-schedules',
      summary: 'Create a scrub schedule',
      tag: 'scrub-schedules',
    },
    {
      method: 'GET',
      url: '/api/v1/scrub-schedules/:name',
      summary: 'Get a scrub schedule',
      tag: 'scrub-schedules',
    },
    {
      method: 'PATCH',
      url: '/api/v1/scrub-schedules/:name',
      summary: 'Update a scrub schedule',
      tag: 'scrub-schedules',
    },
    {
      method: 'DELETE',
      url: '/api/v1/scrub-schedules/:name',
      summary: 'Delete a scrub schedule',
      tag: 'scrub-schedules',
    },
  ]);
}
