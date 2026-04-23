import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/kms-keys.js';
import { registerUnavailable } from './_unavailable.js';

export async function kmsKeysRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/kms-keys', summary: 'List KMS keys', tag: 'kms-keys' },
    { method: 'POST', url: '/api/v1/kms-keys', summary: 'Create a KMS key', tag: 'kms-keys' },
    { method: 'GET', url: '/api/v1/kms-keys/:name', summary: 'Get a KMS key', tag: 'kms-keys' },
    {
      method: 'PATCH',
      url: '/api/v1/kms-keys/:name',
      summary: 'Update a KMS key',
      tag: 'kms-keys',
    },
    {
      method: 'DELETE',
      url: '/api/v1/kms-keys/:name',
      summary: 'Delete a KMS key',
      tag: 'kms-keys',
    },
  ]);
}
