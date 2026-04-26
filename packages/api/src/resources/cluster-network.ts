import { type ClusterNetwork, ClusterNetworkSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildClusterNetworkResource(db: DbClient): PgResource<ClusterNetwork> {
  return new PgResource<ClusterNetwork>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ClusterNetwork',
    schema: ClusterNetworkSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerSingletonRoutes<ClusterNetwork>({
    app,
    basePath: '/api/v1/cluster-network',
    tag: 'cluster-network',
    kind: 'ClusterNetwork',
    resource: buildClusterNetworkResource(db),
    schema: ClusterNetworkSchema,
  });
}
