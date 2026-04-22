import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function vmRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/vms', summary: 'List VMs', tag: 'vms' },
    { method: 'POST', url: '/api/v1/vms', summary: 'Create a VM', tag: 'vms' },
    { method: 'GET', url: '/api/v1/vms/:id', summary: 'Get a VM', tag: 'vms' },
    { method: 'POST', url: '/api/v1/vms/:id/start', summary: 'Start a VM', tag: 'vms' },
    { method: 'POST', url: '/api/v1/vms/:id/stop', summary: 'Stop a VM', tag: 'vms' },
    { method: 'DELETE', url: '/api/v1/vms/:id', summary: 'Delete a VM', tag: 'vms' },
  ]);
}
