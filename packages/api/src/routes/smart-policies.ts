import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/smart-policies.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function smartPoliciesRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/smart-policies', summary: 'List SmartPolicys', tag: 'smart-policies' },
    { method: 'POST', url: '/api/v1/smart-policies', summary: 'Create a SmartPolicy', tag: 'smart-policies' },
    { method: 'GET', url: '/api/v1/smart-policies/:name', summary: 'Get a SmartPolicy', tag: 'smart-policies' },
    { method: 'PATCH', url: '/api/v1/smart-policies/:name', summary: 'Update a SmartPolicy', tag: 'smart-policies' },
    { method: 'DELETE', url: '/api/v1/smart-policies/:name', summary: 'Delete a SmartPolicy', tag: 'smart-policies' },
  ]);
}
