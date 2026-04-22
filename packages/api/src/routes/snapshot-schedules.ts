import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/snapshot-schedules.js';
import { registerStubs } from './_stubs.js';

export async function snapshotSchedulesRoutes(
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
      url: '/api/v1/snapshot-schedules',
      summary: 'List snapshot schedules',
      tag: 'snapshot-schedules',
    },
    {
      method: 'POST',
      url: '/api/v1/snapshot-schedules',
      summary: 'Create a snapshot schedule',
      tag: 'snapshot-schedules',
    },
    {
      method: 'GET',
      url: '/api/v1/snapshot-schedules/:name',
      summary: 'Get a snapshot schedule',
      tag: 'snapshot-schedules',
    },
    {
      method: 'PATCH',
      url: '/api/v1/snapshot-schedules/:name',
      summary: 'Update a snapshot schedule',
      tag: 'snapshot-schedules',
    },
    {
      method: 'DELETE',
      url: '/api/v1/snapshot-schedules/:name',
      summary: 'Delete a snapshot schedule',
      tag: 'snapshot-schedules',
    },
  ]);
}
