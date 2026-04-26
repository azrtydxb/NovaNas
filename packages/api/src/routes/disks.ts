import type { FastifyInstance } from 'fastify';
import { register as registerDisks } from '../resources/disks.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function diskRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerDisks(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/disks', summary: 'List physical disks', tag: 'disks' },
    { method: 'POST', url: '/api/v1/disks', summary: 'Register a disk', tag: 'disks' },
    { method: 'GET', url: '/api/v1/disks/:name', summary: 'Get a disk', tag: 'disks' },
    { method: 'PATCH', url: '/api/v1/disks/:name', summary: 'Update disk metadata', tag: 'disks' },
    { method: 'DELETE', url: '/api/v1/disks/:name', summary: 'Remove a disk', tag: 'disks' },
  ]);
}
