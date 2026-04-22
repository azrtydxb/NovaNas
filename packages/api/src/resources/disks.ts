import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Disk, DiskSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildDiskResource(api: CustomObjectsApi): CrdResource<Disk> {
  return new CrdResource<Disk>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'disks' },
    schema: DiskSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Disk>({
    app,
    basePath: '/api/v1/disks',
    tag: 'disks',
    kind: 'Disk',
    resource: buildDiskResource(api),
    schema: DiskSchema,
  });
}
