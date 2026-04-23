import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/physical-interfaces.js';
import { registerUnavailable } from './_unavailable.js';

export async function physicalInterfacesRoutes(
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
      url: '/api/v1/physical-interfaces',
      summary: 'List physical interfaces',
      tag: 'physical-interfaces',
    },
    {
      method: 'GET',
      url: '/api/v1/physical-interfaces/:name',
      summary: 'Get a physical interface',
      tag: 'physical-interfaces',
    },
  ]);
}
