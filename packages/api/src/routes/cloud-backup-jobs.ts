import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/cloud-backup-jobs.js';
import { registerStubs } from './_stubs.js';

export async function cloudBackupJobsRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerStubs(app, [
    {
      method: 'GET',
      url: '/api/v1/cloud-backup-jobs',
      summary: 'List cloud backup jobs',
      tag: 'cloud-backup-jobs',
    },
    {
      method: 'POST',
      url: '/api/v1/cloud-backup-jobs',
      summary: 'Create a cloud backup job',
      tag: 'cloud-backup-jobs',
    },
    {
      method: 'GET',
      url: '/api/v1/cloud-backup-jobs/:name',
      summary: 'Get a cloud backup job',
      tag: 'cloud-backup-jobs',
    },
    {
      method: 'PATCH',
      url: '/api/v1/cloud-backup-jobs/:name',
      summary: 'Update a cloud backup job',
      tag: 'cloud-backup-jobs',
    },
    {
      method: 'DELETE',
      url: '/api/v1/cloud-backup-jobs/:name',
      summary: 'Delete a cloud backup job',
      tag: 'cloud-backup-jobs',
    },
  ]);
}
