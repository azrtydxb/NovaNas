import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/scrub-schedules.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function scrubSchedulesRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/scrub-schedules', summary: 'List ScrubSchedules', tag: 'scrub-schedules' },
    { method: 'POST', url: '/api/v1/scrub-schedules', summary: 'Create a ScrubSchedule', tag: 'scrub-schedules' },
    { method: 'GET', url: '/api/v1/scrub-schedules/:name', summary: 'Get a ScrubSchedule', tag: 'scrub-schedules' },
    { method: 'PATCH', url: '/api/v1/scrub-schedules/:name', summary: 'Update a ScrubSchedule', tag: 'scrub-schedules' },
    { method: 'DELETE', url: '/api/v1/scrub-schedules/:name', summary: 'Delete a ScrubSchedule', tag: 'scrub-schedules' },
  ]);
}
