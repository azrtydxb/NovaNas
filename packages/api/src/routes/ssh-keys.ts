import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/ssh-keys.js';
import { registerStubs } from './_stubs.js';

export async function sshKeysRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/ssh-keys', summary: 'List SSH keys', tag: 'ssh-keys' },
    { method: 'POST', url: '/api/v1/ssh-keys', summary: 'Create a SSH key', tag: 'ssh-keys' },
    { method: 'GET', url: '/api/v1/ssh-keys/:name', summary: 'Get a SSH key', tag: 'ssh-keys' },
    {
      method: 'PATCH',
      url: '/api/v1/ssh-keys/:name',
      summary: 'Update a SSH key',
      tag: 'ssh-keys',
    },
    {
      method: 'DELETE',
      url: '/api/v1/ssh-keys/:name',
      summary: 'Delete a SSH key',
      tag: 'ssh-keys',
    },
  ]);
}
