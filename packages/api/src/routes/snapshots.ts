import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function snapshotRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/snapshots', summary: 'List snapshots', tag: 'snapshots' },
    { method: 'POST', url: '/api/v1/snapshots', summary: 'Take a snapshot', tag: 'snapshots' },
    { method: 'GET', url: '/api/v1/snapshots/:id', summary: 'Get a snapshot', tag: 'snapshots' },
    {
      method: 'POST',
      url: '/api/v1/snapshots/:id/rollback',
      summary: 'Roll back',
      tag: 'snapshots',
    },
    {
      method: 'DELETE',
      url: '/api/v1/snapshots/:id',
      summary: 'Delete a snapshot',
      tag: 'snapshots',
    },
  ]);
}
