import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerObjectStores } from '../resources/object-stores.js';
import { registerUnavailable } from './_unavailable.js';

export async function objectStoreRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerObjectStores(app, api);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/object-stores',
      summary: 'List object stores',
      tag: 'object-stores',
    },
    {
      method: 'POST',
      url: '/api/v1/object-stores',
      summary: 'Create an object store',
      tag: 'object-stores',
    },
    {
      method: 'GET',
      url: '/api/v1/object-stores/:name',
      summary: 'Get an object store',
      tag: 'object-stores',
    },
    {
      method: 'PATCH',
      url: '/api/v1/object-stores/:name',
      summary: 'Update an object store',
      tag: 'object-stores',
    },
    {
      method: 'DELETE',
      url: '/api/v1/object-stores/:name',
      summary: 'Delete an object store',
      tag: 'object-stores',
    },
  ]);
}
