import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/system-settings.js';
import { registerStubs } from './_stubs.js';

export async function systemSettingsRoutes(
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
      url: '/api/v1/system/settings',
      summary: 'Get system settings',
      tag: 'system-settings',
    },
    {
      method: 'PATCH',
      url: '/api/v1/system/settings',
      summary: 'Update system settings',
      tag: 'system-settings',
    },
  ]);
}
