import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/snapshots.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function snapshotRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/snapshots', summary: 'List Snapshots', tag: 'snapshots' },
    { method: 'POST', url: '/api/v1/snapshots', summary: 'Create a Snapshot', tag: 'snapshots' },
    { method: 'GET', url: '/api/v1/snapshots/:name', summary: 'Get a Snapshot', tag: 'snapshots' },
    { method: 'PATCH', url: '/api/v1/snapshots/:name', summary: 'Update a Snapshot', tag: 'snapshots' },
    { method: 'DELETE', url: '/api/v1/snapshots/:name', summary: 'Delete a Snapshot', tag: 'snapshots' },
  ]);
}
