import { type RemoteAccessTunnel, RemoteAccessTunnelSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildRemoteAccessTunnelResource(db: DbClient): PgResource<RemoteAccessTunnel> {
  return new PgResource<RemoteAccessTunnel>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'RemoteAccessTunnel',
    schema: RemoteAccessTunnelSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<RemoteAccessTunnel>({
    app,
    basePath: '/api/v1/remote-access-tunnels',
    tag: 'remote-access-tunnels',
    kind: 'RemoteAccessTunnel',
    resource: buildRemoteAccessTunnelResource(db),
    schema: RemoteAccessTunnelSchema,
  });
}
