import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type RemoteAccessTunnel, RemoteAccessTunnelSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildRemoteAccessTunnelResource(
  api: CustomObjectsApi
): CrdResource<RemoteAccessTunnel> {
  return new CrdResource<RemoteAccessTunnel>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'remoteaccesstunnels' },
    schema: RemoteAccessTunnelSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<RemoteAccessTunnel>({
    app,
    basePath: '/api/v1/remote-access-tunnels',
    tag: 'remote-access-tunnels',
    kind: 'RemoteAccessTunnel',
    resource: buildRemoteAccessTunnelResource(api),
    schema: RemoteAccessTunnelSchema,
  });
}
