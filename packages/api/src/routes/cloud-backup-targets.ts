import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/cloud-backup-targets.js';
import { registerUnavailable } from './_unavailable.js';

export async function cloudBackupTargetsRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/cloud-backup-targets',
      summary: 'List cloud backup targets',
      tag: 'cloud-backup-targets',
    },
    {
      method: 'POST',
      url: '/api/v1/cloud-backup-targets',
      summary: 'Create a cloud backup target',
      tag: 'cloud-backup-targets',
    },
    {
      method: 'GET',
      url: '/api/v1/cloud-backup-targets/:name',
      summary: 'Get a cloud backup target',
      tag: 'cloud-backup-targets',
    },
    {
      method: 'PATCH',
      url: '/api/v1/cloud-backup-targets/:name',
      summary: 'Update a cloud backup target',
      tag: 'cloud-backup-targets',
    },
    {
      method: 'DELETE',
      url: '/api/v1/cloud-backup-targets/:name',
      summary: 'Delete a cloud backup target',
      tag: 'cloud-backup-targets',
    },
  ]);
}
