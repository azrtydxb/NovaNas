import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerPools } from '../resources/pools.js';
import { registerStubs } from './_stubs.js';

export async function poolRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerPools(app, api);
    return;
  }
  // TODO(wave-10): remove once kube client is mandatory
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/pools', summary: 'List ZFS pools', tag: 'pools' },
    { method: 'POST', url: '/api/v1/pools', summary: 'Create a pool', tag: 'pools' },
    { method: 'GET', url: '/api/v1/pools/:name', summary: 'Get a pool', tag: 'pools' },
    { method: 'PATCH', url: '/api/v1/pools/:name', summary: 'Update a pool', tag: 'pools' },
    { method: 'DELETE', url: '/api/v1/pools/:name', summary: 'Destroy a pool', tag: 'pools' },
  ]);
}
