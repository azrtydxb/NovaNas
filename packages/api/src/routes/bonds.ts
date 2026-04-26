import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/bonds.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function bondsRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/bonds', summary: 'List Bonds', tag: 'bonds' },
    { method: 'POST', url: '/api/v1/bonds', summary: 'Create a Bond', tag: 'bonds' },
    { method: 'GET', url: '/api/v1/bonds/:name', summary: 'Get a Bond', tag: 'bonds' },
    { method: 'PATCH', url: '/api/v1/bonds/:name', summary: 'Update a Bond', tag: 'bonds' },
    { method: 'DELETE', url: '/api/v1/bonds/:name', summary: 'Delete a Bond', tag: 'bonds' },
  ]);
}
