import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/system-settings.js';
import { registerUnavailable } from './_unavailable.js';

export async function systemSettingsRoutes(
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
