import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/slos.js';
import { registerUnavailable } from './_unavailable.js';

export async function slosRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/slos', summary: 'List service level objectives', tag: 'slos' },
    {
      method: 'POST',
      url: '/api/v1/slos',
      summary: 'Create a service level objective',
      tag: 'slos',
    },
    {
      method: 'GET',
      url: '/api/v1/slos/:name',
      summary: 'Get a service level objective',
      tag: 'slos',
    },
    {
      method: 'PATCH',
      url: '/api/v1/slos/:name',
      summary: 'Update a service level objective',
      tag: 'slos',
    },
    {
      method: 'DELETE',
      url: '/api/v1/slos/:name',
      summary: 'Delete a service level objective',
      tag: 'slos',
    },
  ]);
}
