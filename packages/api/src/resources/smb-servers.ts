import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type SmbServer, SmbServerSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildSmbServerResource(api: CustomObjectsApi): CrdResource<SmbServer> {
  return new CrdResource<SmbServer>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'smbservers' },
    schema: SmbServerSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<SmbServer>({
    app,
    basePath: '/api/v1/smb-servers',
    tag: 'smb-servers',
    kind: 'SmbServer',
    resource: buildSmbServerResource(api),
    schema: SmbServerSchema,
  });
}
