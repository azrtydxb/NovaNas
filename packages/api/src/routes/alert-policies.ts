import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/alert-policies.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function alertPoliciesRoutes(
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
      url: '/api/v1/alert-policies',
      summary: 'List AlertPolicys',
      tag: 'alert-policies',
    },
    {
      method: 'POST',
      url: '/api/v1/alert-policies',
      summary: 'Create a AlertPolicy',
      tag: 'alert-policies',
    },
    {
      method: 'GET',
      url: '/api/v1/alert-policies/:name',
      summary: 'Get a AlertPolicy',
      tag: 'alert-policies',
    },
    {
      method: 'PATCH',
      url: '/api/v1/alert-policies/:name',
      summary: 'Update a AlertPolicy',
      tag: 'alert-policies',
    },
    {
      method: 'DELETE',
      url: '/api/v1/alert-policies/:name',
      summary: 'Delete a AlertPolicy',
      tag: 'alert-policies',
    },
  ]);
}
