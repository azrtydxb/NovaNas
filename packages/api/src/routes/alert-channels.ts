import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/alert-channels.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function alertChannelsRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/alert-channels', summary: 'List AlertChannels', tag: 'alert-channels' },
    { method: 'POST', url: '/api/v1/alert-channels', summary: 'Create a AlertChannel', tag: 'alert-channels' },
    { method: 'GET', url: '/api/v1/alert-channels/:name', summary: 'Get a AlertChannel', tag: 'alert-channels' },
    { method: 'PATCH', url: '/api/v1/alert-channels/:name', summary: 'Update a AlertChannel', tag: 'alert-channels' },
    { method: 'DELETE', url: '/api/v1/alert-channels/:name', summary: 'Delete a AlertChannel', tag: 'alert-channels' },
  ]);
}
