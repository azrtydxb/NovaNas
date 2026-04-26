import { type KeycloakRealm, KeycloakRealmSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildKeycloakRealmResource(db: DbClient): PgResource<KeycloakRealm> {
  return new PgResource<KeycloakRealm>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'KeycloakRealm',
    schema: KeycloakRealmSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<KeycloakRealm>({
    app,
    basePath: '/api/v1/keycloak-realms',
    tag: 'keycloak-realms',
    kind: 'KeycloakRealm',
    resource: buildKeycloakRealmResource(db),
    schema: KeycloakRealmSchema,
  });
}
