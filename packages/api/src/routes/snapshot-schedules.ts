import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/snapshot-schedules.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function snapshotSchedulesRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/snapshot-schedules', summary: 'List SnapshotSchedules', tag: 'snapshot-schedules' },
    { method: 'POST', url: '/api/v1/snapshot-schedules', summary: 'Create a SnapshotSchedule', tag: 'snapshot-schedules' },
    { method: 'GET', url: '/api/v1/snapshot-schedules/:name', summary: 'Get a SnapshotSchedule', tag: 'snapshot-schedules' },
    { method: 'PATCH', url: '/api/v1/snapshot-schedules/:name', summary: 'Update a SnapshotSchedule', tag: 'snapshot-schedules' },
    { method: 'DELETE', url: '/api/v1/snapshot-schedules/:name', summary: 'Delete a SnapshotSchedule', tag: 'snapshot-schedules' },
  ]);
}
