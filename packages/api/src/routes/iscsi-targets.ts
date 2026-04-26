import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/iscsi-targets.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function iscsiTargetRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/iscsi-targets',
      summary: 'List IscsiTargets',
      tag: 'iscsi-targets',
    },
    {
      method: 'POST',
      url: '/api/v1/iscsi-targets',
      summary: 'Create a IscsiTarget',
      tag: 'iscsi-targets',
    },
    {
      method: 'GET',
      url: '/api/v1/iscsi-targets/:name',
      summary: 'Get a IscsiTarget',
      tag: 'iscsi-targets',
    },
    {
      method: 'PATCH',
      url: '/api/v1/iscsi-targets/:name',
      summary: 'Update a IscsiTarget',
      tag: 'iscsi-targets',
    },
    {
      method: 'DELETE',
      url: '/api/v1/iscsi-targets/:name',
      summary: 'Delete a IscsiTarget',
      tag: 'iscsi-targets',
    },
  ]);
}
