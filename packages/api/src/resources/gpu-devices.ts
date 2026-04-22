import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type GpuDevice, GpuDeviceSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerReadOnlyRoutes } from './_register_extras.js';

export function buildGpuDeviceResource(api: CustomObjectsApi): CrdResource<GpuDevice> {
  return new CrdResource<GpuDevice>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'gpudevices' },
    schema: GpuDeviceSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerReadOnlyRoutes<GpuDevice>({
    app,
    basePath: '/api/v1/gpu-devices',
    tag: 'gpu-devices',
    kind: 'GpuDevice',
    resource: buildGpuDeviceResource(api),
    schema: GpuDeviceSchema,
  });
}
