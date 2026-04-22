import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/config-backup-policy.js';
import { registerStubs } from './_stubs.js';

export async function configBackupPolicyRoutes(
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
      url: '/api/v1/config-backup-policy',
      summary: 'Get config backup policy',
      tag: 'config-backup-policy',
    },
    {
      method: 'PATCH',
      url: '/api/v1/config-backup-policy',
      summary: 'Update config backup policy',
      tag: 'config-backup-policy',
    },
  ]);
}
