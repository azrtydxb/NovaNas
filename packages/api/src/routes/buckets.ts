import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function bucketRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/buckets', summary: 'List S3 buckets', tag: 'buckets' },
    { method: 'POST', url: '/api/v1/buckets', summary: 'Create a bucket', tag: 'buckets' },
    { method: 'GET', url: '/api/v1/buckets/:name', summary: 'Get a bucket', tag: 'buckets' },
    { method: 'DELETE', url: '/api/v1/buckets/:name', summary: 'Delete a bucket', tag: 'buckets' },
  ]);
}
