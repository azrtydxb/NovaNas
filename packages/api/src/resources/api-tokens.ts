import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type ApiToken, ApiTokenSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildApiTokenResource(api: CustomObjectsApi): CrdResource<ApiToken> {
  return new CrdResource<ApiToken>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'apitokens' },
    schema: ApiTokenSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<ApiToken>({
    app,
    basePath: '/api/v1/api-tokens',
    tag: 'api-tokens',
    kind: 'ApiToken',
    resource: buildApiTokenResource(api),
    schema: ApiTokenSchema,
  });
}
