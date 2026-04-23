import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/ups-policy.js';
import { registerUnavailable } from './_unavailable.js';

export async function upsPolicyRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/ups-policy', summary: 'Get UPS policy', tag: 'ups-policy' },
    { method: 'PATCH', url: '/api/v1/ups-policy', summary: 'Update UPS policy', tag: 'ups-policy' },
  ]);
}
