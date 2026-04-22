import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type NfsServer, NfsServerSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildNfsServerResource(api: CustomObjectsApi): CrdResource<NfsServer> {
  return new CrdResource<NfsServer>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'nfsservers' },
    schema: NfsServerSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<NfsServer>({
    app,
    basePath: '/api/v1/nfs-servers',
    tag: 'nfs-servers',
    kind: 'NfsServer',
    resource: buildNfsServerResource(api),
    schema: NfsServerSchema,
  });
}
