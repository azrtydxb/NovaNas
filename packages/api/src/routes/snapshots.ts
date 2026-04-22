import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerSnapshots } from '../resources/snapshots.js';
import { registerStubs } from './_stubs.js';

export async function snapshotRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerSnapshots(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/snapshots', summary: 'List snapshots', tag: 'snapshots' },
    { method: 'POST', url: '/api/v1/snapshots', summary: 'Take a snapshot', tag: 'snapshots' },
    {
      method: 'GET',
      url: '/api/v1/snapshots/:name',
      summary: 'Get a snapshot',
      tag: 'snapshots',
    },
    {
      method: 'PATCH',
      url: '/api/v1/snapshots/:name',
      summary: 'Update a snapshot',
      tag: 'snapshots',
    },
    {
      method: 'DELETE',
      url: '/api/v1/snapshots/:name',
      summary: 'Delete a snapshot',
      tag: 'snapshots',
    },
  ]);
}
