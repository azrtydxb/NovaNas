import type { FastifyInstance } from 'fastify';
import { register as registerPools } from '../resources/pools.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function poolRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerPools(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/pools', summary: 'List ZFS pools', tag: 'pools' },
    { method: 'POST', url: '/api/v1/pools', summary: 'Create a pool', tag: 'pools' },
    { method: 'GET', url: '/api/v1/pools/:name', summary: 'Get a pool', tag: 'pools' },
    { method: 'PATCH', url: '/api/v1/pools/:name', summary: 'Update a pool', tag: 'pools' },
    { method: 'DELETE', url: '/api/v1/pools/:name', summary: 'Destroy a pool', tag: 'pools' },
  ]);
}
