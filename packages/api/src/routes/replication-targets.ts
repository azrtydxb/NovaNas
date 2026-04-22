import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/replication-targets.js';
import { registerStubs } from './_stubs.js';

export async function replicationTargetsRoutes(
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
      url: '/api/v1/replication-targets',
      summary: 'List replication targets',
      tag: 'replication-targets',
    },
    {
      method: 'POST',
      url: '/api/v1/replication-targets',
      summary: 'Create a replication target',
      tag: 'replication-targets',
    },
    {
      method: 'GET',
      url: '/api/v1/replication-targets/:name',
      summary: 'Get a replication target',
      tag: 'replication-targets',
    },
    {
      method: 'PATCH',
      url: '/api/v1/replication-targets/:name',
      summary: 'Update a replication target',
      tag: 'replication-targets',
    },
    {
      method: 'DELETE',
      url: '/api/v1/replication-targets/:name',
      summary: 'Delete a replication target',
      tag: 'replication-targets',
    },
  ]);
}
