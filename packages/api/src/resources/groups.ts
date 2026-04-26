import { type Group, GroupSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import type { KeycloakAdmin } from '../services/keycloak-admin.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildGroupResource(db: DbClient): PgResource<Group> {
  return new PgResource<Group>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Group',
    schema: GroupSchema,
    namespaced: false,
  });
}

export function register(
  app: FastifyInstance,
  db: DbClient,
  keycloakAdmin?: KeycloakAdmin | null
): void {
  registerCrudRoutes<Group>({
    app,
    basePath: '/api/v1/groups',
    tag: 'groups',
    kind: 'Group',
    resource: buildGroupResource(db),
    schema: GroupSchema,
    // Keycloak group sync (#51). Hooks are best-effort: failure is
    // logged in _register.ts but does not roll back the Postgres
    // write. Convergence on transient failure is owned by the planned
    // retry queue (also #51).
    afterCreate: keycloakAdmin
      ? async (group, req) => {
          const id = await keycloakAdmin.ensureGroup({
            name: group.spec.name,
            realm: group.spec.realm,
            members: group.spec.members,
          });
          req.log.debug(
            { kind: 'Group', name: group.metadata.name, keycloakId: id },
            'group synced to keycloak'
          );
        }
      : undefined,
    afterDelete: keycloakAdmin
      ? async (name, req) => {
          // Spec is already gone by afterDelete; the resource name
          // matches the Keycloak group name (the controller keyed
          // deletion off req.Name too).
          await keycloakAdmin.deleteGroup('', name);
          req.log.debug({ kind: 'Group', name }, 'group removed from keycloak');
        }
      : undefined,
  });
}
