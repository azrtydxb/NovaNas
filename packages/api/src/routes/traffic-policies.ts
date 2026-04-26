import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/traffic-policies.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function trafficPoliciesRoutes(
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
