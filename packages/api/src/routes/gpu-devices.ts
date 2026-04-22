import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/gpu-devices.js';
import { registerStubs } from './_stubs.js';

export async function gpuDevicesRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/gpu-devices', summary: 'List GPU devices', tag: 'gpu-devices' },
    {
      method: 'GET',
      url: '/api/v1/gpu-devices/:name',
      summary: 'Get a GPU device',
      tag: 'gpu-devices',
    },
  ]);
}
