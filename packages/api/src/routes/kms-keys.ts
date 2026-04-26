import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/kms-keys.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function kmsKeysRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/kms-keys', summary: 'List KmsKeys', tag: 'kms-keys' },
    { method: 'POST', url: '/api/v1/kms-keys', summary: 'Create a KmsKey', tag: 'kms-keys' },
    { method: 'GET', url: '/api/v1/kms-keys/:name', summary: 'Get a KmsKey', tag: 'kms-keys' },
    { method: 'PATCH', url: '/api/v1/kms-keys/:name', summary: 'Update a KmsKey', tag: 'kms-keys' },
    {
      method: 'DELETE',
      url: '/api/v1/kms-keys/:name',
      summary: 'Delete a KmsKey',
      tag: 'kms-keys',
    },
  ]);
}
