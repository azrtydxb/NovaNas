import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/slos.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function slosRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/slos', summary: 'List ServiceLevelObjectives', tag: 'slos' },
    { method: 'POST', url: '/api/v1/slos', summary: 'Create a ServiceLevelObjective', tag: 'slos' },
    {
      method: 'GET',
      url: '/api/v1/slos/:name',
      summary: 'Get a ServiceLevelObjective',
      tag: 'slos',
    },
    {
      method: 'PATCH',
      url: '/api/v1/slos/:name',
      summary: 'Update a ServiceLevelObjective',
      tag: 'slos',
    },
    {
      method: 'DELETE',
      url: '/api/v1/slos/:name',
      summary: 'Delete a ServiceLevelObjective',
      tag: 'slos',
    },
  ]);
}
