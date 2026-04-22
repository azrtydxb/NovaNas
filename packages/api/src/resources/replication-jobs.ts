import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type ReplicationJob, ReplicationJobSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildReplicationJobResource(api: CustomObjectsApi): CrdResource<ReplicationJob> {
  return new CrdResource<ReplicationJob>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'replicationjobs' },
    schema: ReplicationJobSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<ReplicationJob>({
    app,
    basePath: '/api/v1/replication-jobs',
    tag: 'replication-jobs',
    kind: 'ReplicationJob',
    resource: buildReplicationJobResource(api),
    schema: ReplicationJobSchema,
  });
}
