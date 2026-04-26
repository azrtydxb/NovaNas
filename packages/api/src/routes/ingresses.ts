import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/ingresses.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function ingressesRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/ingresses', summary: 'List ingresses', tag: 'ingresses' },
    { method: 'POST', url: '/api/v1/ingresses', summary: 'Create an ingress', tag: 'ingresses' },
    { method: 'GET', url: '/api/v1/ingresses/:name', summary: 'Get an ingress', tag: 'ingresses' },
    {
      method: 'PATCH',
      url: '/api/v1/ingresses/:name',
      summary: 'Update an ingress',
      tag: 'ingresses',
    },
    {
      method: 'DELETE',
      url: '/api/v1/ingresses/:name',
      summary: 'Delete an ingress',
      tag: 'ingresses',
    },
  ]);
}
