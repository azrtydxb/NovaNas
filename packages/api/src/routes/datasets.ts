import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function datasetRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/datasets', summary: 'List datasets', tag: 'datasets' },
    { method: 'POST', url: '/api/v1/datasets', summary: 'Create a dataset', tag: 'datasets' },
    { method: 'GET', url: '/api/v1/datasets/:id', summary: 'Get a dataset', tag: 'datasets' },
    { method: 'PATCH', url: '/api/v1/datasets/:id', summary: 'Update a dataset', tag: 'datasets' },
    { method: 'DELETE', url: '/api/v1/datasets/:id', summary: 'Delete a dataset', tag: 'datasets' },
  ]);
}
