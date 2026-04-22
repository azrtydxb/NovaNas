import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type User, UserSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildUserResource(api: CustomObjectsApi): CrdResource<User> {
  return new CrdResource<User>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'users' },
    schema: UserSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<User>({
    app,
    basePath: '/api/v1/users',
    tag: 'users',
    kind: 'User',
    resource: buildUserResource(api),
    schema: UserSchema,
  });
}
