import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/firewall-rules.js';
import { registerStubs } from './_stubs.js';

export async function firewallRulesRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerStubs(app, [
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
