import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerDisks } from '../resources/disks.js';
import { registerStubs } from './_stubs.js';

export async function diskRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerDisks(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/disks', summary: 'List physical disks', tag: 'disks' },
    { method: 'POST', url: '/api/v1/disks', summary: 'Register a disk', tag: 'disks' },
    { method: 'GET', url: '/api/v1/disks/:name', summary: 'Get a disk', tag: 'disks' },
    { method: 'PATCH', url: '/api/v1/disks/:name', summary: 'Update disk metadata', tag: 'disks' },
    { method: 'DELETE', url: '/api/v1/disks/:name', summary: 'Remove a disk', tag: 'disks' },
  ]);
}
