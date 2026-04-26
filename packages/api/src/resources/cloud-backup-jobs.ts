import { type CloudBackupJob, CloudBackupJobSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildCloudBackupJobResource(db: DbClient): PgResource<CloudBackupJob> {
  return new PgResource<CloudBackupJob>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'CloudBackupJob',
    schema: CloudBackupJobSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<CloudBackupJob>({
    app,
    basePath: '/api/v1/cloud-backup-jobs',
    tag: 'cloud-backup-jobs',
    kind: 'CloudBackupJob',
    resource: buildCloudBackupJobResource(db),
    schema: CloudBackupJobSchema,
  });
}
