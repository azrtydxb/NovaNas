import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerDatasets } from '../resources/datasets.js';
import { registerUnavailable } from './_unavailable.js';

export async function datasetRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerDatasets(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/datasets', summary: 'List datasets', tag: 'datasets' },
    { method: 'POST', url: '/api/v1/datasets', summary: 'Create a dataset', tag: 'datasets' },
    { method: 'GET', url: '/api/v1/datasets/:name', summary: 'Get a dataset', tag: 'datasets' },
    {
      method: 'PATCH',
      url: '/api/v1/datasets/:name',
      summary: 'Update a dataset',
      tag: 'datasets',
    },
    {
      method: 'DELETE',
      url: '/api/v1/datasets/:name',
      summary: 'Delete a dataset',
      tag: 'datasets',
    },
  ]);
}
