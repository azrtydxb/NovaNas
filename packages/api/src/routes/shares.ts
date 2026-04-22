import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function shareRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/shares', summary: 'List SMB/NFS shares', tag: 'shares' },
    { method: 'POST', url: '/api/v1/shares', summary: 'Create a share', tag: 'shares' },
    { method: 'GET', url: '/api/v1/shares/:id', summary: 'Get a share', tag: 'shares' },
    { method: 'PATCH', url: '/api/v1/shares/:id', summary: 'Update a share', tag: 'shares' },
    { method: 'DELETE', url: '/api/v1/shares/:id', summary: 'Delete a share', tag: 'shares' },
  ]);
}
