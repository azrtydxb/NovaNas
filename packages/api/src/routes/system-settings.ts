import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/system-settings.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function systemSettingsRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/system-settings', summary: 'List SystemSettingss', tag: 'system-settings' },
    { method: 'POST', url: '/api/v1/system-settings', summary: 'Create a SystemSettings', tag: 'system-settings' },
    { method: 'GET', url: '/api/v1/system-settings/:name', summary: 'Get a SystemSettings', tag: 'system-settings' },
    { method: 'PATCH', url: '/api/v1/system-settings/:name', summary: 'Update a SystemSettings', tag: 'system-settings' },
    { method: 'DELETE', url: '/api/v1/system-settings/:name', summary: 'Delete a SystemSettings', tag: 'system-settings' },
  ]);
}
