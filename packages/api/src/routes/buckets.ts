import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/buckets.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function bucketRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/buckets', summary: 'List Buckets', tag: 'buckets' },
    { method: 'POST', url: '/api/v1/buckets', summary: 'Create a Bucket', tag: 'buckets' },
    { method: 'GET', url: '/api/v1/buckets/:name', summary: 'Get a Bucket', tag: 'buckets' },
    { method: 'PATCH', url: '/api/v1/buckets/:name', summary: 'Update a Bucket', tag: 'buckets' },
    { method: 'DELETE', url: '/api/v1/buckets/:name', summary: 'Delete a Bucket', tag: 'buckets' },
  ]);
}
