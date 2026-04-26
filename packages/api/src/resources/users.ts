import { type User, UserSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import type { KeycloakAdmin } from '../services/keycloak-admin.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildUserResource(db: DbClient): PgResource<User> {
  return new PgResource<User>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'User',
    schema: UserSchema,
    namespaced: false,
  });
}

export function register(
  app: FastifyInstance,
  db: DbClient,
  keycloakAdmin?: KeycloakAdmin | null
): void {
  registerCrudRoutes<User>({
    app,
    basePath: '/api/v1/users',
    tag: 'users',
    kind: 'User',
    resource: buildUserResource(db),
    schema: UserSchema,
    // Keycloak user sync (#51). The original operator also provisioned
    // a per-tenant OpenBao policy + kubernetes-auth role; that's
    // tracked under a separate identity-provisioning pass — see the
    // risk notes on #51.
    afterCreate: keycloakAdmin
      ? async (user, req) => {
          const id = await keycloakAdmin.ensureUser({
            username: user.spec.username,
            realm: user.spec.realm,
            email: user.spec.email,
            enabled: user.spec.enabled ?? true,
            groups: user.spec.groups,
          });
          req.log.debug(
            { kind: 'User', name: user.metadata.name, keycloakId: id },
            'user synced to keycloak'
          );
        }
      : undefined,
    afterPatch: keycloakAdmin
      ? async (user, _patch, req) => {
          await keycloakAdmin.ensureUser({
            username: user.spec.username,
            realm: user.spec.realm,
            email: user.spec.email,
            enabled: user.spec.enabled ?? true,
            groups: user.spec.groups,
          });
          req.log.debug({ kind: 'User', name: user.metadata.name }, 'user updated in keycloak');
        }
      : undefined,
    afterDelete: keycloakAdmin
      ? async (name, req) => {
          // Spec is gone by afterDelete; the resource name matches
          // the username fallback the operator used.
          await keycloakAdmin.deleteUser('', name);
          req.log.debug({ kind: 'User', name }, 'user removed from keycloak');
        }
      : undefined,
  });
}
