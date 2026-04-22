import type { FastifyInstance } from 'fastify';
import { registerStubs } from './_stubs.js';

export async function poolRoutes(app: FastifyInstance): Promise<void> {
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/pools', summary: 'List ZFS pools', tag: 'pools' },
    { method: 'POST', url: '/api/v1/pools', summary: 'Create a pool', tag: 'pools' },
    { method: 'GET', url: '/api/v1/pools/:name', summary: 'Get a pool', tag: 'pools' },
    { method: 'DELETE', url: '/api/v1/pools/:name', summary: 'Destroy a pool', tag: 'pools' },
    { method: 'POST', url: '/api/v1/pools/:name/scrub', summary: 'Start a scrub', tag: 'pools' },
  ]);
}
