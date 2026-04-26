import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/update-policy.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function updatePolicyRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/update-policy', summary: 'List UpdatePolicys', tag: 'update-policy' },
    { method: 'POST', url: '/api/v1/update-policy', summary: 'Create a UpdatePolicy', tag: 'update-policy' },
    { method: 'GET', url: '/api/v1/update-policy/:name', summary: 'Get a UpdatePolicy', tag: 'update-policy' },
    { method: 'PATCH', url: '/api/v1/update-policy/:name', summary: 'Update a UpdatePolicy', tag: 'update-policy' },
    { method: 'DELETE', url: '/api/v1/update-policy/:name', summary: 'Delete a UpdatePolicy', tag: 'update-policy' },
  ]);
}
