import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type SshKey, SshKeySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildSshKeyResource(api: CustomObjectsApi): CrdResource<SshKey> {
  return new CrdResource<SshKey>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'sshkeys' },
    schema: SshKeySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<SshKey>({
    app,
    basePath: '/api/v1/ssh-keys',
    tag: 'ssh-keys',
    kind: 'SshKey',
    resource: buildSshKeyResource(api),
    schema: SshKeySchema,
  });
}
