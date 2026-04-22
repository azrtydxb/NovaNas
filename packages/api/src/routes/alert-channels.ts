import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/alert-channels.js';
import { registerStubs } from './_stubs.js';

export async function alertChannelsRoutes(
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
      url: '/api/v1/alert-channels',
      summary: 'List alert channels',
      tag: 'alert-channels',
    },
    {
      method: 'POST',
      url: '/api/v1/alert-channels',
      summary: 'Create an alert channel',
      tag: 'alert-channels',
    },
    {
      method: 'GET',
      url: '/api/v1/alert-channels/:name',
      summary: 'Get an alert channel',
      tag: 'alert-channels',
    },
    {
      method: 'PATCH',
      url: '/api/v1/alert-channels/:name',
      summary: 'Update an alert channel',
      tag: 'alert-channels',
    },
    {
      method: 'DELETE',
      url: '/api/v1/alert-channels/:name',
      summary: 'Delete an alert channel',
      tag: 'alert-channels',
    },
  ]);
}
