import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/keycloak-realms.js';
import type { DbClient } from '../services/db.js';
import type { KeycloakAdmin } from '../services/keycloak-admin.js';
import { registerUnavailable } from './_unavailable.js';

export async function keycloakRealmsRoutes(
  app: FastifyInstance,
  db?: DbClient | null,
  keycloakAdmin?: KeycloakAdmin | null
): Promise<void> {
  if (db) {
    registerImpl(app, db, keycloakAdmin ?? null);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/keycloak-realms',
      summary: 'List KeycloakRealms',
      tag: 'keycloak-realms',
    },
    {
      method: 'POST',
      url: '/api/v1/keycloak-realms',
      summary: 'Create a KeycloakRealm',
      tag: 'keycloak-realms',
    },
    {
      method: 'GET',
      url: '/api/v1/keycloak-realms/:name',
      summary: 'Get a KeycloakRealm',
      tag: 'keycloak-realms',
    },
    {
      method: 'PATCH',
      url: '/api/v1/keycloak-realms/:name',
      summary: 'Update a KeycloakRealm',
      tag: 'keycloak-realms',
    },
    {
      method: 'DELETE',
      url: '/api/v1/keycloak-realms/:name',
      summary: 'Delete a KeycloakRealm',
      tag: 'keycloak-realms',
    },
  ]);
}
