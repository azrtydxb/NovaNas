import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/alert-policies.js';
import { registerUnavailable } from './_unavailable.js';

export async function alertPoliciesRoutes(
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
      url: '/api/v1/alert-policies',
      summary: 'List alert policies',
      tag: 'alert-policies',
    },
    {
      method: 'POST',
      url: '/api/v1/alert-policies',
      summary: 'Create an alert policy',
      tag: 'alert-policies',
    },
    {
      method: 'GET',
      url: '/api/v1/alert-policies/:name',
      summary: 'Get an alert policy',
      tag: 'alert-policies',
    },
    {
      method: 'PATCH',
      url: '/api/v1/alert-policies/:name',
      summary: 'Update an alert policy',
      tag: 'alert-policies',
    },
    {
      method: 'DELETE',
      url: '/api/v1/alert-policies/:name',
      summary: 'Delete an alert policy',
      tag: 'alert-policies',
    },
  ]);
}
