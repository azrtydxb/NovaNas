import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/ups-policy.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function upsPolicyRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/ups-policy', summary: 'List UpsPolicys', tag: 'ups-policy' },
    { method: 'POST', url: '/api/v1/ups-policy', summary: 'Create a UpsPolicy', tag: 'ups-policy' },
    { method: 'GET', url: '/api/v1/ups-policy/:name', summary: 'Get a UpsPolicy', tag: 'ups-policy' },
    { method: 'PATCH', url: '/api/v1/ups-policy/:name', summary: 'Update a UpsPolicy', tag: 'ups-policy' },
    { method: 'DELETE', url: '/api/v1/ups-policy/:name', summary: 'Delete a UpsPolicy', tag: 'ups-policy' },
  ]);
}
