import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/api-tokens.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function apiTokensRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/api-tokens', summary: 'List ApiTokens', tag: 'api-tokens' },
    { method: 'POST', url: '/api/v1/api-tokens', summary: 'Create a ApiToken', tag: 'api-tokens' },
    {
      method: 'GET',
      url: '/api/v1/api-tokens/:name',
      summary: 'Get a ApiToken',
      tag: 'api-tokens',
    },
    {
      method: 'PATCH',
      url: '/api/v1/api-tokens/:name',
      summary: 'Update a ApiToken',
      tag: 'api-tokens',
    },
    {
      method: 'DELETE',
      url: '/api/v1/api-tokens/:name',
      summary: 'Delete a ApiToken',
      tag: 'api-tokens',
    },
  ]);
}
