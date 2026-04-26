import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/config-backup-policy.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function configBackupPolicyRoutes(
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
      url: '/api/v1/config-backup-policy',
      summary: 'List ConfigBackupPolicys',
      tag: 'config-backup-policy',
    },
    {
      method: 'POST',
      url: '/api/v1/config-backup-policy',
      summary: 'Create a ConfigBackupPolicy',
      tag: 'config-backup-policy',
    },
    {
      method: 'GET',
      url: '/api/v1/config-backup-policy/:name',
      summary: 'Get a ConfigBackupPolicy',
      tag: 'config-backup-policy',
    },
    {
      method: 'PATCH',
      url: '/api/v1/config-backup-policy/:name',
      summary: 'Update a ConfigBackupPolicy',
      tag: 'config-backup-policy',
    },
    {
      method: 'DELETE',
      url: '/api/v1/config-backup-policy/:name',
      summary: 'Delete a ConfigBackupPolicy',
      tag: 'config-backup-policy',
    },
  ]);
}
