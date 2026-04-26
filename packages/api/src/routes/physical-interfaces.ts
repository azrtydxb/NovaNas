import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/physical-interfaces.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function physicalInterfacesRoutes(
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
      url: '/api/v1/physical-interfaces',
      summary: 'List PhysicalInterfaces',
      tag: 'physical-interfaces',
    },
    {
      method: 'POST',
      url: '/api/v1/physical-interfaces',
      summary: 'Create a PhysicalInterface',
      tag: 'physical-interfaces',
    },
    {
      method: 'GET',
      url: '/api/v1/physical-interfaces/:name',
      summary: 'Get a PhysicalInterface',
      tag: 'physical-interfaces',
    },
    {
      method: 'PATCH',
      url: '/api/v1/physical-interfaces/:name',
      summary: 'Update a PhysicalInterface',
      tag: 'physical-interfaces',
    },
    {
      method: 'DELETE',
      url: '/api/v1/physical-interfaces/:name',
      summary: 'Delete a PhysicalInterface',
      tag: 'physical-interfaces',
    },
  ]);
}
