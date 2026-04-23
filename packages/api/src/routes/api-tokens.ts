import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/api-tokens.js';
import { registerUnavailable } from './_unavailable.js';

export async function apiTokensRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/api-tokens', summary: 'List API tokens', tag: 'api-tokens' },
    {
      method: 'POST',
      url: '/api/v1/api-tokens',
      summary: 'Create an API token',
      tag: 'api-tokens',
    },
    {
      method: 'GET',
      url: '/api/v1/api-tokens/:name',
      summary: 'Get an API token',
      tag: 'api-tokens',
    },
    {
      method: 'PATCH',
      url: '/api/v1/api-tokens/:name',
      summary: 'Update an API token',
      tag: 'api-tokens',
    },
    {
      method: 'DELETE',
      url: '/api/v1/api-tokens/:name',
      summary: 'Delete an API token',
      tag: 'api-tokens',
    },
  ]);
}
