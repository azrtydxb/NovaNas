import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerAppsAvailable } from '../resources/apps-available.js';
import { registerUnavailable } from './_unavailable.js';

export async function appsAvailableRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerAppsAvailable(app, api);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/apps-available',
      summary: 'List available apps',
      tag: 'apps-available',
    },
    {
      method: 'GET',
      url: '/api/v1/apps-available/:name',
      summary: 'Get an available app',
      tag: 'apps-available',
    },
  ]);
}
