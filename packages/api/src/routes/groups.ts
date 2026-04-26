import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/groups.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function groupsRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/groups', summary: 'List Groups', tag: 'groups' },
    { method: 'POST', url: '/api/v1/groups', summary: 'Create a Group', tag: 'groups' },
    { method: 'GET', url: '/api/v1/groups/:name', summary: 'Get a Group', tag: 'groups' },
    { method: 'PATCH', url: '/api/v1/groups/:name', summary: 'Update a Group', tag: 'groups' },
    { method: 'DELETE', url: '/api/v1/groups/:name', summary: 'Delete a Group', tag: 'groups' },
  ]);
}
