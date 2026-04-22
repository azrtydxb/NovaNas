import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/update-policy.js';
import { registerStubs } from './_stubs.js';

export async function updatePolicyRoutes(
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
      url: '/api/v1/update-policy',
      summary: 'Get update policy',
      tag: 'update-policy',
    },
    {
      method: 'PATCH',
      url: '/api/v1/update-policy',
      summary: 'Update update policy',
      tag: 'update-policy',
    },
  ]);
}
