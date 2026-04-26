import { type SshKey, SshKeySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildSshKeyResource(db: DbClient): PgResource<SshKey> {
  return new PgResource<SshKey>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'SshKey',
    schema: SshKeySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<SshKey>({
    app,
    basePath: '/api/v1/ssh-keys',
    tag: 'ssh-keys',
    kind: 'SshKey',
    resource: buildSshKeyResource(db),
    schema: SshKeySchema,
  });
}
