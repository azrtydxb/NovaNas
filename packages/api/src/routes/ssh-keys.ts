import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/ssh-keys.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function sshKeysRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/ssh-keys', summary: 'List SshKeys', tag: 'ssh-keys' },
    { method: 'POST', url: '/api/v1/ssh-keys', summary: 'Create a SshKey', tag: 'ssh-keys' },
    { method: 'GET', url: '/api/v1/ssh-keys/:name', summary: 'Get a SshKey', tag: 'ssh-keys' },
    { method: 'PATCH', url: '/api/v1/ssh-keys/:name', summary: 'Update a SshKey', tag: 'ssh-keys' },
    {
      method: 'DELETE',
      url: '/api/v1/ssh-keys/:name',
      summary: 'Delete a SshKey',
      tag: 'ssh-keys',
    },
  ]);
}
