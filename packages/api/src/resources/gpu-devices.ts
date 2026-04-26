import { type GpuDevice, GpuDeviceSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerReadOnlyRoutes } from './_register_extras.js';

export function buildGpuDeviceResource(db: DbClient): PgResource<GpuDevice> {
  return new PgResource<GpuDevice>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'GpuDevice',
    schema: GpuDeviceSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerReadOnlyRoutes<GpuDevice>({
    app,
    basePath: '/api/v1/gpu-devices',
    tag: 'gpu-devices',
    kind: 'GpuDevice',
    resource: buildGpuDeviceResource(db),
    schema: GpuDeviceSchema,
  });
}
