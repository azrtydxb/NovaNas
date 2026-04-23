import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/encryption-policies.js';
import { registerUnavailable } from './_unavailable.js';

export async function encryptionPoliciesRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/encryption-policies',
      summary: 'List encryption policies',
      tag: 'encryption-policies',
    },
    {
      method: 'POST',
      url: '/api/v1/encryption-policies',
      summary: 'Create an encryption policy',
      tag: 'encryption-policies',
    },
    {
      method: 'GET',
      url: '/api/v1/encryption-policies/:name',
      summary: 'Get an encryption policy',
      tag: 'encryption-policies',
    },
    {
      method: 'PATCH',
      url: '/api/v1/encryption-policies/:name',
      summary: 'Update an encryption policy',
      tag: 'encryption-policies',
    },
    {
      method: 'DELETE',
      url: '/api/v1/encryption-policies/:name',
      summary: 'Delete an encryption policy',
      tag: 'encryption-policies',
    },
  ]);
}
