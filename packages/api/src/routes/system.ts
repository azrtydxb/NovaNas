import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function systemRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    {
      method: 'GET',
      url: '/api/v1/system/info',
      summary: 'System info (CPU, RAM, uptime)',
      tag: 'system',
    },
    { method: 'GET', url: '/api/v1/system/network', summary: 'Network interfaces', tag: 'system' },
    { method: 'GET', url: '/api/v1/system/alerts', summary: 'Active alerts', tag: 'system' },
    { method: 'GET', url: '/api/v1/system/events', summary: 'Recent events', tag: 'system' },
    {
      method: 'POST',
      url: '/api/v1/system/reboot',
      summary: 'Reboot the appliance',
      tag: 'system',
    },
    {
      method: 'POST',
      url: '/api/v1/system/shutdown',
      summary: 'Shutdown the appliance',
      tag: 'system',
    },
  ]);
}
