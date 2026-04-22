import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerIsoLibraries } from '../resources/iso-libraries.js';
import { registerStubs } from './_stubs.js';

export async function isoLibraryRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerIsoLibraries(app, api);
    return;
  }
  registerStubs(app, [
    {
      method: 'GET',
      url: '/api/v1/iso-libraries',
      summary: 'List ISO libraries',
      tag: 'iso-libraries',
    },
    {
      method: 'POST',
      url: '/api/v1/iso-libraries',
      summary: 'Create an ISO library',
      tag: 'iso-libraries',
    },
    {
      method: 'GET',
      url: '/api/v1/iso-libraries/:name',
      summary: 'Get an ISO library',
      tag: 'iso-libraries',
    },
    {
      method: 'PATCH',
      url: '/api/v1/iso-libraries/:name',
      summary: 'Update an ISO library',
      tag: 'iso-libraries',
    },
    {
      method: 'DELETE',
      url: '/api/v1/iso-libraries/:name',
      summary: 'Delete an ISO library',
      tag: 'iso-libraries',
    },
  ]);
}
