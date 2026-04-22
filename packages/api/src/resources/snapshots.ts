import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Snapshot, SnapshotSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildSnapshotResource(api: CustomObjectsApi): CrdResource<Snapshot> {
  return new CrdResource<Snapshot>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'snapshots' },
    schema: SnapshotSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Snapshot>({
    app,
    basePath: '/api/v1/snapshots',
    tag: 'snapshots',
    kind: 'Snapshot',
    resource: buildSnapshotResource(api),
    schema: SnapshotSchema,
  });
}
