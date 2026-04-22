import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/replication-jobs.js';
import { registerStubs } from './_stubs.js';

export async function replicationJobsRoutes(
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
      url: '/api/v1/replication-jobs',
      summary: 'List replication jobs',
      tag: 'replication-jobs',
    },
    {
      method: 'POST',
      url: '/api/v1/replication-jobs',
      summary: 'Create a replication job',
      tag: 'replication-jobs',
    },
    {
      method: 'GET',
      url: '/api/v1/replication-jobs/:name',
      summary: 'Get a replication job',
      tag: 'replication-jobs',
    },
    {
      method: 'PATCH',
      url: '/api/v1/replication-jobs/:name',
      summary: 'Update a replication job',
      tag: 'replication-jobs',
    },
    {
      method: 'DELETE',
      url: '/api/v1/replication-jobs/:name',
      summary: 'Delete a replication job',
      tag: 'replication-jobs',
    },
  ]);
}
