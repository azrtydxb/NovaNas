import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type CloudBackupJob, CloudBackupJobSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildCloudBackupJobResource(api: CustomObjectsApi): CrdResource<CloudBackupJob> {
  return new CrdResource<CloudBackupJob>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'cloudbackupjobs' },
    schema: CloudBackupJobSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<CloudBackupJob>({
    app,
    basePath: '/api/v1/cloud-backup-jobs',
    tag: 'cloud-backup-jobs',
    kind: 'CloudBackupJob',
    resource: buildCloudBackupJobResource(api),
    schema: CloudBackupJobSchema,
  });
}
