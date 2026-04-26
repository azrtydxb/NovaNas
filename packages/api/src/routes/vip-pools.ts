import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/vip-pools.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function vipPoolsRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/vip-pools', summary: 'List VipPools', tag: 'vip-pools' },
    { method: 'POST', url: '/api/v1/vip-pools', summary: 'Create a VipPool', tag: 'vip-pools' },
    { method: 'GET', url: '/api/v1/vip-pools/:name', summary: 'Get a VipPool', tag: 'vip-pools' },
    {
      method: 'PATCH',
      url: '/api/v1/vip-pools/:name',
      summary: 'Update a VipPool',
      tag: 'vip-pools',
    },
    {
      method: 'DELETE',
      url: '/api/v1/vip-pools/:name',
      summary: 'Delete a VipPool',
      tag: 'vip-pools',
    },
  ]);
}
