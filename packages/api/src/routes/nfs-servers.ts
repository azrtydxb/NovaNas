import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/nfs-servers.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function nfsServerRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/nfs-servers', summary: 'List NfsServers', tag: 'nfs-servers' },
    {
      method: 'POST',
      url: '/api/v1/nfs-servers',
      summary: 'Create a NfsServer',
      tag: 'nfs-servers',
    },
    {
      method: 'GET',
      url: '/api/v1/nfs-servers/:name',
      summary: 'Get a NfsServer',
      tag: 'nfs-servers',
    },
    {
      method: 'PATCH',
      url: '/api/v1/nfs-servers/:name',
      summary: 'Update a NfsServer',
      tag: 'nfs-servers',
    },
    {
      method: 'DELETE',
      url: '/api/v1/nfs-servers/:name',
      summary: 'Delete a NfsServer',
      tag: 'nfs-servers',
    },
  ]);
}
