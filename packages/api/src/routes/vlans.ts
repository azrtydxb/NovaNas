import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/vlans.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function vlansRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/vlans', summary: 'List Vlans', tag: 'vlans' },
    { method: 'POST', url: '/api/v1/vlans', summary: 'Create a Vlan', tag: 'vlans' },
    { method: 'GET', url: '/api/v1/vlans/:name', summary: 'Get a Vlan', tag: 'vlans' },
    { method: 'PATCH', url: '/api/v1/vlans/:name', summary: 'Update a Vlan', tag: 'vlans' },
    { method: 'DELETE', url: '/api/v1/vlans/:name', summary: 'Delete a Vlan', tag: 'vlans' },
  ]);
}
