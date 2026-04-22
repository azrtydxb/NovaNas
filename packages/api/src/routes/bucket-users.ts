import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerBucketUsers } from '../resources/bucket-users.js';
import { registerStubs } from './_stubs.js';

export async function bucketUserRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerBucketUsers(app, api);
    return;
  }
  registerStubs(app, [
    {
      method: 'GET',
      url: '/api/v1/bucket-users',
      summary: 'List bucket users',
      tag: 'bucket-users',
    },
    {
      method: 'POST',
      url: '/api/v1/bucket-users',
      summary: 'Create a bucket user',
      tag: 'bucket-users',
    },
    {
      method: 'GET',
      url: '/api/v1/bucket-users/:name',
      summary: 'Get a bucket user',
      tag: 'bucket-users',
    },
    {
      method: 'PATCH',
      url: '/api/v1/bucket-users/:name',
      summary: 'Update a bucket user',
      tag: 'bucket-users',
    },
    {
      method: 'DELETE',
      url: '/api/v1/bucket-users/:name',
      summary: 'Delete a bucket user',
      tag: 'bucket-users',
    },
  ]);
}
