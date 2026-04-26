import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/service-policy.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function servicePolicyRoutes(
  app: FastifyInstance,
  db?: DbClient | null
): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/service-policy',
      summary: 'Get service policy',
      tag: 'service-policy',
    },
    {
      method: 'PATCH',
      url: '/api/v1/service-policy',
      summary: 'Update service policy',
      tag: 'service-policy',
    },
  ]);
}
