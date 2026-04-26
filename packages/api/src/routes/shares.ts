import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/shares.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function shareRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/shares', summary: 'List Shares', tag: 'shares' },
    { method: 'POST', url: '/api/v1/shares', summary: 'Create a Share', tag: 'shares' },
    { method: 'GET', url: '/api/v1/shares/:name', summary: 'Get a Share', tag: 'shares' },
    { method: 'PATCH', url: '/api/v1/shares/:name', summary: 'Update a Share', tag: 'shares' },
    { method: 'DELETE', url: '/api/v1/shares/:name', summary: 'Delete a Share', tag: 'shares' },
  ]);
}
