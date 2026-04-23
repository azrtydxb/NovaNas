import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/vip-pools.js';
import { registerUnavailable } from './_unavailable.js';

export async function vipPoolsRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/vip-pools', summary: 'List VIP pools', tag: 'vip-pools' },
    { method: 'POST', url: '/api/v1/vip-pools', summary: 'Create a VIP pool', tag: 'vip-pools' },
    { method: 'GET', url: '/api/v1/vip-pools/:name', summary: 'Get a VIP pool', tag: 'vip-pools' },
    {
      method: 'PATCH',
      url: '/api/v1/vip-pools/:name',
      summary: 'Update a VIP pool',
      tag: 'vip-pools',
    },
    {
      method: 'DELETE',
      url: '/api/v1/vip-pools/:name',
      summary: 'Delete a VIP pool',
      tag: 'vip-pools',
    },
  ]);
}
