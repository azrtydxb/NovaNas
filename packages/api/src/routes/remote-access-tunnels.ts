import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/remote-access-tunnels.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function remoteAccessTunnelsRoutes(
  app: FastifyInstance,
  db?: DbClient | null
): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/remote-access-tunnels',
      summary: 'List remote access tunnels',
      tag: 'remote-access-tunnels',
    },
    {
      method: 'POST',
      url: '/api/v1/remote-access-tunnels',
      summary: 'Create a remote access tunnel',
      tag: 'remote-access-tunnels',
    },
    {
      method: 'GET',
      url: '/api/v1/remote-access-tunnels/:name',
      summary: 'Get a remote access tunnel',
      tag: 'remote-access-tunnels',
    },
    {
      method: 'PATCH',
      url: '/api/v1/remote-access-tunnels/:name',
      summary: 'Update a remote access tunnel',
      tag: 'remote-access-tunnels',
    },
    {
      method: 'DELETE',
      url: '/api/v1/remote-access-tunnels/:name',
      summary: 'Delete a remote access tunnel',
      tag: 'remote-access-tunnels',
    },
  ]);
}
