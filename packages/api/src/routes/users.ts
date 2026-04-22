import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function userRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/users', summary: 'List users', tag: 'users' },
    { method: 'POST', url: '/api/v1/users', summary: 'Create a user', tag: 'users' },
    { method: 'GET', url: '/api/v1/users/:id', summary: 'Get a user', tag: 'users' },
    { method: 'PATCH', url: '/api/v1/users/:id', summary: 'Update a user', tag: 'users' },
    { method: 'DELETE', url: '/api/v1/users/:id', summary: 'Delete a user', tag: 'users' },
    { method: 'GET', url: '/api/v1/groups', summary: 'List groups', tag: 'users' },
  ]);
}
