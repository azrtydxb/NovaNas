import { type CloudBackupTarget, CloudBackupTargetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildCloudBackupTargetResource(db: DbClient): PgResource<CloudBackupTarget> {
  return new PgResource<CloudBackupTarget>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'CloudBackupTarget',
    schema: CloudBackupTargetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<CloudBackupTarget>({
    app,
    basePath: '/api/v1/cloud-backup-targets',
    tag: 'cloud-backup-targets',
    kind: 'CloudBackupTarget',
    resource: buildCloudBackupTargetResource(db),
    schema: CloudBackupTargetSchema,
  });
}
