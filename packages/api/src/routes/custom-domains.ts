import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/custom-domains.js';
import { registerStubs } from './_stubs.js';

export async function customDomainsRoutes(
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
      url: '/api/v1/custom-domains',
      summary: 'List custom domains',
      tag: 'custom-domains',
    },
    {
      method: 'POST',
      url: '/api/v1/custom-domains',
      summary: 'Create a custom domain',
      tag: 'custom-domains',
    },
    {
      method: 'GET',
      url: '/api/v1/custom-domains/:name',
      summary: 'Get a custom domain',
      tag: 'custom-domains',
    },
    {
      method: 'PATCH',
      url: '/api/v1/custom-domains/:name',
      summary: 'Update a custom domain',
      tag: 'custom-domains',
    },
    {
      method: 'DELETE',
      url: '/api/v1/custom-domains/:name',
      summary: 'Delete a custom domain',
      tag: 'custom-domains',
    },
  ]);
}
