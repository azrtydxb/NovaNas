import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerUsers } from '../resources/users.js';
import { registerStubs } from './_stubs.js';

export async function userRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerUsers(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/users', summary: 'List users', tag: 'users' },
    { method: 'POST', url: '/api/v1/users', summary: 'Create a user', tag: 'users' },
    { method: 'GET', url: '/api/v1/users/:name', summary: 'Get a user', tag: 'users' },
    { method: 'PATCH', url: '/api/v1/users/:name', summary: 'Update a user', tag: 'users' },
    { method: 'DELETE', url: '/api/v1/users/:name', summary: 'Delete a user', tag: 'users' },
  ]);
}
