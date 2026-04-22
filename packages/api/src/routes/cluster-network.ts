import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/cluster-network.js';
import { registerStubs } from './_stubs.js';

export async function clusterNetworkRoutes(
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
      url: '/api/v1/cluster-network',
      summary: 'Get cluster network',
      tag: 'cluster-network',
    },
    {
      method: 'PATCH',
      url: '/api/v1/cluster-network',
      summary: 'Update cluster network',
      tag: 'cluster-network',
    },
  ]);
}
