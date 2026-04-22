import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/certificates.js';
import { registerStubs } from './_stubs.js';

export async function certificatesRoutes(
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
      url: '/api/v1/certificates',
      summary: 'List certificates',
      tag: 'certificates',
    },
    {
      method: 'POST',
      url: '/api/v1/certificates',
      summary: 'Create a certificate',
      tag: 'certificates',
    },
    {
      method: 'GET',
      url: '/api/v1/certificates/:name',
      summary: 'Get a certificate',
      tag: 'certificates',
    },
    {
      method: 'PATCH',
      url: '/api/v1/certificates/:name',
      summary: 'Update a certificate',
      tag: 'certificates',
    },
    {
      method: 'DELETE',
      url: '/api/v1/certificates/:name',
      summary: 'Delete a certificate',
      tag: 'certificates',
    },
  ]);
}
