import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/smart-policies.js';
import { registerStubs } from './_stubs.js';

export async function smartPoliciesRoutes(
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
      url: '/api/v1/smart-policies',
      summary: 'List SMART policies',
      tag: 'smart-policies',
    },
    {
      method: 'POST',
      url: '/api/v1/smart-policies',
      summary: 'Create a SMART policy',
      tag: 'smart-policies',
    },
    {
      method: 'GET',
      url: '/api/v1/smart-policies/:name',
      summary: 'Get a SMART policy',
      tag: 'smart-policies',
    },
    {
      method: 'PATCH',
      url: '/api/v1/smart-policies/:name',
      summary: 'Update a SMART policy',
      tag: 'smart-policies',
    },
    {
      method: 'DELETE',
      url: '/api/v1/smart-policies/:name',
      summary: 'Delete a SMART policy',
      tag: 'smart-policies',
    },
  ]);
}
