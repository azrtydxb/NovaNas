import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerVms } from '../resources/vms.js';
import { registerStubs } from './_stubs.js';

export async function vmRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerVms(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/vms', summary: 'List VMs', tag: 'vms' },
    { method: 'POST', url: '/api/v1/vms', summary: 'Create a VM', tag: 'vms' },
    { method: 'GET', url: '/api/v1/vms/:name', summary: 'Get a VM', tag: 'vms' },
    { method: 'PATCH', url: '/api/v1/vms/:name', summary: 'Update a VM', tag: 'vms' },
    { method: 'DELETE', url: '/api/v1/vms/:name', summary: 'Delete a VM', tag: 'vms' },
  ]);
}
