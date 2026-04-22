import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/keycloak-realms.js';
import { registerStubs } from './_stubs.js';

export async function keycloakRealmsRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerStubs(app, [
    {
      method: 'GET',
      url: '/api/v1/keycloak-realms',
      summary: 'List Keycloak realms',
      tag: 'keycloak-realms',
    },
    {
      method: 'POST',
      url: '/api/v1/keycloak-realms',
      summary: 'Create a Keycloak realm',
      tag: 'keycloak-realms',
    },
    {
      method: 'GET',
      url: '/api/v1/keycloak-realms/:name',
      summary: 'Get a Keycloak realm',
      tag: 'keycloak-realms',
    },
    {
      method: 'PATCH',
      url: '/api/v1/keycloak-realms/:name',
      summary: 'Update a Keycloak realm',
      tag: 'keycloak-realms',
    },
    {
      method: 'DELETE',
      url: '/api/v1/keycloak-realms/:name',
      summary: 'Delete a Keycloak realm',
      tag: 'keycloak-realms',
    },
  ]);
}
