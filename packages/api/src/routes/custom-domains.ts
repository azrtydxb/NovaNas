import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/custom-domains.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function customDomainsRoutes(
  app: FastifyInstance,
  db?: DbClient | null
): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/custom-domains',
      summary: 'List CustomDomains',
      tag: 'custom-domains',
    },
    {
      method: 'POST',
      url: '/api/v1/custom-domains',
      summary: 'Create a CustomDomain',
      tag: 'custom-domains',
    },
    {
      method: 'GET',
      url: '/api/v1/custom-domains/:name',
      summary: 'Get a CustomDomain',
      tag: 'custom-domains',
    },
    {
      method: 'PATCH',
      url: '/api/v1/custom-domains/:name',
      summary: 'Update a CustomDomain',
      tag: 'custom-domains',
    },
    {
      method: 'DELETE',
      url: '/api/v1/custom-domains/:name',
      summary: 'Delete a CustomDomain',
      tag: 'custom-domains',
    },
  ]);
}
