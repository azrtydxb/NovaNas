import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/iso-libraries.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function isoLibraryRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/iso-libraries', summary: 'List IsoLibrarys', tag: 'iso-libraries' },
    { method: 'POST', url: '/api/v1/iso-libraries', summary: 'Create a IsoLibrary', tag: 'iso-libraries' },
    { method: 'GET', url: '/api/v1/iso-libraries/:name', summary: 'Get a IsoLibrary', tag: 'iso-libraries' },
    { method: 'PATCH', url: '/api/v1/iso-libraries/:name', summary: 'Update a IsoLibrary', tag: 'iso-libraries' },
    { method: 'DELETE', url: '/api/v1/iso-libraries/:name', summary: 'Delete a IsoLibrary', tag: 'iso-libraries' },
  ]);
}
