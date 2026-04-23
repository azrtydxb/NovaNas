import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/bonds.js';
import { registerUnavailable } from './_unavailable.js';

export async function bondsRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/bonds', summary: 'List bonds', tag: 'bonds' },
    { method: 'POST', url: '/api/v1/bonds', summary: 'Create a bond', tag: 'bonds' },
    { method: 'GET', url: '/api/v1/bonds/:name', summary: 'Get a bond', tag: 'bonds' },
    { method: 'PATCH', url: '/api/v1/bonds/:name', summary: 'Update a bond', tag: 'bonds' },
    { method: 'DELETE', url: '/api/v1/bonds/:name', summary: 'Delete a bond', tag: 'bonds' },
  ]);
}
