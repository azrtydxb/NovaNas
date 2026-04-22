import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type ClusterNetwork, ClusterNetworkSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildClusterNetworkResource(api: CustomObjectsApi): CrdResource<ClusterNetwork> {
  return new CrdResource<ClusterNetwork>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'clusternetworks' },
    schema: ClusterNetworkSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerSingletonRoutes<ClusterNetwork>({
    app,
    basePath: '/api/v1/cluster-network',
    tag: 'cluster-network',
    kind: 'ClusterNetwork',
    resource: buildClusterNetworkResource(api),
    schema: ClusterNetworkSchema,
  });
}
