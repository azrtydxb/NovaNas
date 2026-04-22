import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/traffic-policies.js';
import { registerStubs } from './_stubs.js';

export async function trafficPoliciesRoutes(
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
      url: '/api/v1/traffic-policies',
      summary: 'List traffic policies',
      tag: 'traffic-policies',
    },
    {
      method: 'POST',
      url: '/api/v1/traffic-policies',
      summary: 'Create a traffic policy',
      tag: 'traffic-policies',
    },
    {
      method: 'GET',
      url: '/api/v1/traffic-policies/:name',
      summary: 'Get a traffic policy',
      tag: 'traffic-policies',
    },
    {
      method: 'PATCH',
      url: '/api/v1/traffic-policies/:name',
      summary: 'Update a traffic policy',
      tag: 'traffic-policies',
    },
    {
      method: 'DELETE',
      url: '/api/v1/traffic-policies/:name',
      summary: 'Delete a traffic policy',
      tag: 'traffic-policies',
    },
  ]);
}
