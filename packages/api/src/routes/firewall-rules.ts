import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/firewall-rules.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function firewallRulesRoutes(
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
      url: '/api/v1/firewall-rules',
      summary: 'List firewall rules',
      tag: 'firewall-rules',
    },
    {
      method: 'POST',
      url: '/api/v1/firewall-rules',
      summary: 'Create a firewall rule',
      tag: 'firewall-rules',
    },
    {
      method: 'GET',
      url: '/api/v1/firewall-rules/:name',
      summary: 'Get a firewall rule',
      tag: 'firewall-rules',
    },
    {
      method: 'PATCH',
      url: '/api/v1/firewall-rules/:name',
      summary: 'Update a firewall rule',
      tag: 'firewall-rules',
    },
    {
      method: 'DELETE',
      url: '/api/v1/firewall-rules/:name',
      summary: 'Delete a firewall rule',
      tag: 'firewall-rules',
    },
  ]);
}
