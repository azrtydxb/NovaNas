import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function diskRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/disks', summary: 'List physical disks', tag: 'disks' },
    { method: 'GET', url: '/api/v1/disks/:id', summary: 'Get a disk (SMART data)', tag: 'disks' },
    { method: 'POST', url: '/api/v1/disks/:id/wipe', summary: 'Wipe a disk', tag: 'disks' },
  ]);
}
