import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/groups.js';
import { registerStubs } from './_stubs.js';

export async function groupsRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/groups', summary: 'List groups', tag: 'groups' },
    { method: 'POST', url: '/api/v1/groups', summary: 'Create a group', tag: 'groups' },
    { method: 'GET', url: '/api/v1/groups/:name', summary: 'Get a group', tag: 'groups' },
    { method: 'PATCH', url: '/api/v1/groups/:name', summary: 'Update a group', tag: 'groups' },
    { method: 'DELETE', url: '/api/v1/groups/:name', summary: 'Delete a group', tag: 'groups' },
  ]);
}
