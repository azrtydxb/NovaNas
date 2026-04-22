import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/service-policy.js';
import { registerStubs } from './_stubs.js';

export async function servicePolicyRoutes(
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
      url: '/api/v1/service-policy',
      summary: 'Get service policy',
      tag: 'service-policy',
    },
    {
      method: 'PATCH',
      url: '/api/v1/service-policy',
      summary: 'Update service policy',
      tag: 'service-policy',
    },
  ]);
}
