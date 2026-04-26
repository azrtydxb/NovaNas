import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/bucket-users.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function bucketUserRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/bucket-users', summary: 'List BucketUsers', tag: 'bucket-users' },
    { method: 'POST', url: '/api/v1/bucket-users', summary: 'Create a BucketUser', tag: 'bucket-users' },
    { method: 'GET', url: '/api/v1/bucket-users/:name', summary: 'Get a BucketUser', tag: 'bucket-users' },
    { method: 'PATCH', url: '/api/v1/bucket-users/:name', summary: 'Update a BucketUser', tag: 'bucket-users' },
    { method: 'DELETE', url: '/api/v1/bucket-users/:name', summary: 'Delete a BucketUser', tag: 'bucket-users' },
  ]);
}
