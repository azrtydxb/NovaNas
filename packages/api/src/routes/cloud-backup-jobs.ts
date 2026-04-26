import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/cloud-backup-jobs.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function cloudBackupJobsRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/cloud-backup-jobs', summary: 'List CloudBackupJobs', tag: 'cloud-backup-jobs' },
    { method: 'POST', url: '/api/v1/cloud-backup-jobs', summary: 'Create a CloudBackupJob', tag: 'cloud-backup-jobs' },
    { method: 'GET', url: '/api/v1/cloud-backup-jobs/:name', summary: 'Get a CloudBackupJob', tag: 'cloud-backup-jobs' },
    { method: 'PATCH', url: '/api/v1/cloud-backup-jobs/:name', summary: 'Update a CloudBackupJob', tag: 'cloud-backup-jobs' },
    { method: 'DELETE', url: '/api/v1/cloud-backup-jobs/:name', summary: 'Delete a CloudBackupJob', tag: 'cloud-backup-jobs' },
  ]);
}
