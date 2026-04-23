import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/host-interfaces.js';
import { registerUnavailable } from './_unavailable.js';

export async function hostInterfacesRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/host-interfaces',
      summary: 'List host interfaces',
      tag: 'host-interfaces',
    },
    {
      method: 'POST',
      url: '/api/v1/host-interfaces',
      summary: 'Create a host interface',
      tag: 'host-interfaces',
    },
    {
      method: 'GET',
      url: '/api/v1/host-interfaces/:name',
      summary: 'Get a host interface',
      tag: 'host-interfaces',
    },
    {
      method: 'PATCH',
      url: '/api/v1/host-interfaces/:name',
      summary: 'Update a host interface',
      tag: 'host-interfaces',
    },
    {
      method: 'DELETE',
      url: '/api/v1/host-interfaces/:name',
      summary: 'Delete a host interface',
      tag: 'host-interfaces',
    },
  ]);
}
