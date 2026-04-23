import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/vlans.js';
import { registerUnavailable } from './_unavailable.js';

export async function vlansRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/vlans', summary: 'List VLANs', tag: 'vlans' },
    { method: 'POST', url: '/api/v1/vlans', summary: 'Create a VLAN', tag: 'vlans' },
    { method: 'GET', url: '/api/v1/vlans/:name', summary: 'Get a VLAN', tag: 'vlans' },
    { method: 'PATCH', url: '/api/v1/vlans/:name', summary: 'Update a VLAN', tag: 'vlans' },
    { method: 'DELETE', url: '/api/v1/vlans/:name', summary: 'Delete a VLAN', tag: 'vlans' },
  ]);
}
