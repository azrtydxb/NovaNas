import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/users.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function userRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/users', summary: 'List Users', tag: 'users' },
    { method: 'POST', url: '/api/v1/users', summary: 'Create a User', tag: 'users' },
    { method: 'GET', url: '/api/v1/users/:name', summary: 'Get a User', tag: 'users' },
    { method: 'PATCH', url: '/api/v1/users/:name', summary: 'Update a User', tag: 'users' },
    { method: 'DELETE', url: '/api/v1/users/:name', summary: 'Delete a User', tag: 'users' },
  ]);
}
