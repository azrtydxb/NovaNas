import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type KeycloakRealm, KeycloakRealmSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildKeycloakRealmResource(api: CustomObjectsApi): CrdResource<KeycloakRealm> {
  return new CrdResource<KeycloakRealm>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'keycloakrealms' },
    schema: KeycloakRealmSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<KeycloakRealm>({
    app,
    basePath: '/api/v1/keycloak-realms',
    tag: 'keycloak-realms',
    kind: 'KeycloakRealm',
    resource: buildKeycloakRealmResource(api),
    schema: KeycloakRealmSchema,
  });
}
