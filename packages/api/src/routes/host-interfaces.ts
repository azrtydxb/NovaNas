import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/host-interfaces.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function hostInterfacesRoutes(
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
      url: '/api/v1/host-interfaces',
      summary: 'List HostInterfaces',
      tag: 'host-interfaces',
    },
    {
      method: 'POST',
      url: '/api/v1/host-interfaces',
      summary: 'Create a HostInterface',
      tag: 'host-interfaces',
    },
    {
      method: 'GET',
      url: '/api/v1/host-interfaces/:name',
      summary: 'Get a HostInterface',
      tag: 'host-interfaces',
    },
    {
      method: 'PATCH',
      url: '/api/v1/host-interfaces/:name',
      summary: 'Update a HostInterface',
      tag: 'host-interfaces',
    },
    {
      method: 'DELETE',
      url: '/api/v1/host-interfaces/:name',
      summary: 'Delete a HostInterface',
      tag: 'host-interfaces',
    },
  ]);
}
