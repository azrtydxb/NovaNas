import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/object-stores.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function objectStoreRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/object-stores',
      summary: 'List ObjectStores',
      tag: 'object-stores',
    },
    {
      method: 'POST',
      url: '/api/v1/object-stores',
      summary: 'Create a ObjectStore',
      tag: 'object-stores',
    },
    {
      method: 'GET',
      url: '/api/v1/object-stores/:name',
      summary: 'Get a ObjectStore',
      tag: 'object-stores',
    },
    {
      method: 'PATCH',
      url: '/api/v1/object-stores/:name',
      summary: 'Update a ObjectStore',
      tag: 'object-stores',
    },
    {
      method: 'DELETE',
      url: '/api/v1/object-stores/:name',
      summary: 'Delete a ObjectStore',
      tag: 'object-stores',
    },
  ]);
}
