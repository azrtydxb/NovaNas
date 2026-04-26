import { type KeycloakRealm, KeycloakRealmSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import type { KeycloakAdmin } from '../services/keycloak-admin.js';
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

export function register(
  app: FastifyInstance,
  db: DbClient,
  keycloakAdmin?: KeycloakAdmin | null
): void {
  registerCrudRoutes<KeycloakRealm>({
    app,
    basePath: '/api/v1/keycloak-realms',
    tag: 'keycloak-realms',
    kind: 'KeycloakRealm',
    resource: buildKeycloakRealmResource(db),
    schema: KeycloakRealmSchema,
    // Realm sync (#51). Realm CREATION needs master-realm credentials
    // and is owned by the post-install Job; the api can only update
    // mutable fields on an already-bootstrapped realm. afterCreate
    // and afterPatch share the same logic — the underlying call is
    // idempotent.
    afterCreate: keycloakAdmin
      ? async (realm, req) => {
          const updated = await keycloakAdmin.updateRealm(realm.metadata.name, {
            displayName: realm.spec.displayName,
            defaultLocale: realm.spec.defaultLocale,
            passwordPolicy: realm.spec.passwordPolicy,
            enabled: true,
          });
          if (!updated) {
            req.log.warn(
              { kind: 'KeycloakRealm', name: realm.metadata.name },
              'realm not found in keycloak — bootstrap is owned by the post-install Job'
            );
          }
        }
      : undefined,
    afterPatch: keycloakAdmin
      ? async (realm, _patch, req) => {
          await keycloakAdmin.updateRealm(realm.metadata.name, {
            displayName: realm.spec.displayName,
            defaultLocale: realm.spec.defaultLocale,
            passwordPolicy: realm.spec.passwordPolicy,
          });
          req.log.debug(
            { kind: 'KeycloakRealm', name: realm.metadata.name },
            'realm updated in keycloak'
          );
        }
      : undefined,
    // afterDelete intentionally omitted: realm deletion is destructive
    // and rare; do it via a deliberate ops procedure (post-uninstall
    // Job or kcadm), not as a side effect of an api DELETE.
  });
}
