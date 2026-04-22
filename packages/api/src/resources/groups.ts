import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Group, GroupSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildGroupResource(api: CustomObjectsApi): CrdResource<Group> {
  return new CrdResource<Group>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'groups' },
    schema: GroupSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Group>({
    app,
    basePath: '/api/v1/groups',
    tag: 'groups',
    kind: 'Group',
    resource: buildGroupResource(api),
    schema: GroupSchema,
  });
}
