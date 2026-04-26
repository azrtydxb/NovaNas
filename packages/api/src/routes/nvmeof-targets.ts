import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/nvmeof-targets.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function nvmeofTargetRoutes(
  app: FastifyInstance,
  db?: DbClient | null
): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/nvmeof-targets',
      summary: 'List NvmeofTargets',
      tag: 'nvmeof-targets',
    },
    {
      method: 'POST',
      url: '/api/v1/nvmeof-targets',
      summary: 'Create a NvmeofTarget',
      tag: 'nvmeof-targets',
    },
    {
      method: 'GET',
      url: '/api/v1/nvmeof-targets/:name',
      summary: 'Get a NvmeofTarget',
      tag: 'nvmeof-targets',
    },
    {
      method: 'PATCH',
      url: '/api/v1/nvmeof-targets/:name',
      summary: 'Update a NvmeofTarget',
      tag: 'nvmeof-targets',
    },
    {
      method: 'DELETE',
      url: '/api/v1/nvmeof-targets/:name',
      summary: 'Delete a NvmeofTarget',
      tag: 'nvmeof-targets',
    },
  ]);
}
