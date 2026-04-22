import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function appRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/apps', summary: 'List installed apps', tag: 'apps' },
    { method: 'POST', url: '/api/v1/apps', summary: 'Install an app', tag: 'apps' },
    { method: 'GET', url: '/api/v1/apps/:id', summary: 'Get an app', tag: 'apps' },
    { method: 'PATCH', url: '/api/v1/apps/:id', summary: 'Update app config', tag: 'apps' },
    { method: 'DELETE', url: '/api/v1/apps/:id', summary: 'Uninstall an app', tag: 'apps' },
    { method: 'GET', url: '/api/v1/apps/catalog', summary: 'Browse the catalog', tag: 'apps' },
  ]);
}
