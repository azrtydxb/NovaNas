import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerShares } from '../resources/shares.js';
import { registerStubs } from './_stubs.js';

export async function shareRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerShares(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/shares', summary: 'List SMB/NFS shares', tag: 'shares' },
    { method: 'POST', url: '/api/v1/shares', summary: 'Create a share', tag: 'shares' },
    { method: 'GET', url: '/api/v1/shares/:name', summary: 'Get a share', tag: 'shares' },
    { method: 'PATCH', url: '/api/v1/shares/:name', summary: 'Update a share', tag: 'shares' },
    { method: 'DELETE', url: '/api/v1/shares/:name', summary: 'Delete a share', tag: 'shares' },
  ]);
}
