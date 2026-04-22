import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerBuckets } from '../resources/buckets.js';
import { registerStubs } from './_stubs.js';

export async function bucketRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerBuckets(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/buckets', summary: 'List S3 buckets', tag: 'buckets' },
    { method: 'POST', url: '/api/v1/buckets', summary: 'Create a bucket', tag: 'buckets' },
    { method: 'GET', url: '/api/v1/buckets/:name', summary: 'Get a bucket', tag: 'buckets' },
    { method: 'PATCH', url: '/api/v1/buckets/:name', summary: 'Update a bucket', tag: 'buckets' },
    { method: 'DELETE', url: '/api/v1/buckets/:name', summary: 'Delete a bucket', tag: 'buckets' },
  ]);
}
