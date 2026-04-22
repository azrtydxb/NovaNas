import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerApps } from '../resources/apps.js';
import { registerStubs } from './_stubs.js';

export async function appRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerApps(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/apps', summary: 'List installed apps', tag: 'apps' },
    { method: 'POST', url: '/api/v1/apps', summary: 'Install an app', tag: 'apps' },
    { method: 'GET', url: '/api/v1/apps/:name', summary: 'Get an app', tag: 'apps' },
    { method: 'PATCH', url: '/api/v1/apps/:name', summary: 'Update app config', tag: 'apps' },
    { method: 'DELETE', url: '/api/v1/apps/:name', summary: 'Uninstall an app', tag: 'apps' },
  ]);
}
