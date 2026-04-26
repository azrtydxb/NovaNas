import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/datasets.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function datasetRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/datasets', summary: 'List Datasets', tag: 'datasets' },
    { method: 'POST', url: '/api/v1/datasets', summary: 'Create a Dataset', tag: 'datasets' },
    { method: 'GET', url: '/api/v1/datasets/:name', summary: 'Get a Dataset', tag: 'datasets' },
    {
      method: 'PATCH',
      url: '/api/v1/datasets/:name',
      summary: 'Update a Dataset',
      tag: 'datasets',
    },
    {
      method: 'DELETE',
      url: '/api/v1/datasets/:name',
      summary: 'Delete a Dataset',
      tag: 'datasets',
    },
  ]);
}
