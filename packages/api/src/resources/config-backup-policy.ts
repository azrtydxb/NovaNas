import { type ConfigBackupPolicy, ConfigBackupPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildConfigBackupPolicyResource(db: DbClient): PgResource<ConfigBackupPolicy> {
  return new PgResource<ConfigBackupPolicy>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ConfigBackupPolicy',
    schema: ConfigBackupPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<ConfigBackupPolicy>({
    app,
    basePath: '/api/v1/config-backup-policy',
    tag: 'config-backup-policy',
    kind: 'ConfigBackupPolicy',
    resource: buildConfigBackupPolicyResource(db),
    schema: ConfigBackupPolicySchema,
  });
}
