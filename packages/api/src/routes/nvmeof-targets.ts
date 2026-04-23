import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerNvmeofTargets } from '../resources/nvmeof-targets.js';
import { registerUnavailable } from './_unavailable.js';

export async function nvmeofTargetRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerNvmeofTargets(app, api);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/nvmeof-targets',
      summary: 'List NVMe-oF targets',
      tag: 'nvmeof-targets',
    },
    {
      method: 'POST',
      url: '/api/v1/nvmeof-targets',
      summary: 'Create an NVMe-oF target',
      tag: 'nvmeof-targets',
    },
    {
      method: 'GET',
      url: '/api/v1/nvmeof-targets/:name',
      summary: 'Get an NVMe-oF target',
      tag: 'nvmeof-targets',
    },
    {
      method: 'PATCH',
      url: '/api/v1/nvmeof-targets/:name',
      summary: 'Update an NVMe-oF target',
      tag: 'nvmeof-targets',
    },
    {
      method: 'DELETE',
      url: '/api/v1/nvmeof-targets/:name',
      summary: 'Delete an NVMe-oF target',
      tag: 'nvmeof-targets',
    },
  ]);
}
