import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/ingresses.js';
import { registerStubs } from './_stubs.js';

export async function ingressesRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/ingresses', summary: 'List ingresses', tag: 'ingresses' },
    { method: 'POST', url: '/api/v1/ingresses', summary: 'Create an ingress', tag: 'ingresses' },
    { method: 'GET', url: '/api/v1/ingresses/:name', summary: 'Get an ingress', tag: 'ingresses' },
    {
      method: 'PATCH',
      url: '/api/v1/ingresses/:name',
      summary: 'Update an ingress',
      tag: 'ingresses',
    },
    {
      method: 'DELETE',
      url: '/api/v1/ingresses/:name',
      summary: 'Delete an ingress',
      tag: 'ingresses',
    },
  ]);
}
