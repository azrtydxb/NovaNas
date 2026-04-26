import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/certificates.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function certificatesRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/certificates', summary: 'List Certificates', tag: 'certificates' },
    { method: 'POST', url: '/api/v1/certificates', summary: 'Create a Certificate', tag: 'certificates' },
    { method: 'GET', url: '/api/v1/certificates/:name', summary: 'Get a Certificate', tag: 'certificates' },
    { method: 'PATCH', url: '/api/v1/certificates/:name', summary: 'Update a Certificate', tag: 'certificates' },
    { method: 'DELETE', url: '/api/v1/certificates/:name', summary: 'Delete a Certificate', tag: 'certificates' },
  ]);
}
