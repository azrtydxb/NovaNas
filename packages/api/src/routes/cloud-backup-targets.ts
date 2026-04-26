import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/cloud-backup-targets.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function cloudBackupTargetsRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/cloud-backup-targets', summary: 'List CloudBackupTargets', tag: 'cloud-backup-targets' },
    { method: 'POST', url: '/api/v1/cloud-backup-targets', summary: 'Create a CloudBackupTarget', tag: 'cloud-backup-targets' },
    { method: 'GET', url: '/api/v1/cloud-backup-targets/:name', summary: 'Get a CloudBackupTarget', tag: 'cloud-backup-targets' },
    { method: 'PATCH', url: '/api/v1/cloud-backup-targets/:name', summary: 'Update a CloudBackupTarget', tag: 'cloud-backup-targets' },
    { method: 'DELETE', url: '/api/v1/cloud-backup-targets/:name', summary: 'Delete a CloudBackupTarget', tag: 'cloud-backup-targets' },
  ]);
}
