import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/encryption-policies.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function encryptionPoliciesRoutes(
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
      url: '/api/v1/encryption-policies',
      summary: 'List EncryptionPolicys',
      tag: 'encryption-policies',
    },
    {
      method: 'POST',
      url: '/api/v1/encryption-policies',
      summary: 'Create a EncryptionPolicy',
      tag: 'encryption-policies',
    },
    {
      method: 'GET',
      url: '/api/v1/encryption-policies/:name',
      summary: 'Get a EncryptionPolicy',
      tag: 'encryption-policies',
    },
    {
      method: 'PATCH',
      url: '/api/v1/encryption-policies/:name',
      summary: 'Update a EncryptionPolicy',
      tag: 'encryption-policies',
    },
    {
      method: 'DELETE',
      url: '/api/v1/encryption-policies/:name',
      summary: 'Delete a EncryptionPolicy',
      tag: 'encryption-policies',
    },
  ]);
}
