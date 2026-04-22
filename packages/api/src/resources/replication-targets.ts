import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type ReplicationTarget, ReplicationTargetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildReplicationTargetResource(
  api: CustomObjectsApi
): CrdResource<ReplicationTarget> {
  return new CrdResource<ReplicationTarget>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'replicationtargets' },
    schema: ReplicationTargetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<ReplicationTarget>({
    app,
    basePath: '/api/v1/replication-targets',
    tag: 'replication-targets',
    kind: 'ReplicationTarget',
    resource: buildReplicationTargetResource(api),
    schema: ReplicationTargetSchema,
  });
}
