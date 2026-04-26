import type { FastifyInstance } from 'fastify';
import { register as registerAppsAvailable } from '../resources/apps-available.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function appsAvailableRoutes(
  app: FastifyInstance,
  db?: DbClient | null
): Promise<void> {
  if (db) {
    registerAppsAvailable(app, db);
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
