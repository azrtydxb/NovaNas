import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type CloudBackupTarget, CloudBackupTargetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildCloudBackupTargetResource(
  api: CustomObjectsApi
): CrdResource<CloudBackupTarget> {
  return new CrdResource<CloudBackupTarget>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'cloudbackuptargets' },
    schema: CloudBackupTargetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<CloudBackupTarget>({
    app,
    basePath: '/api/v1/cloud-backup-targets',
    tag: 'cloud-backup-targets',
    kind: 'CloudBackupTarget',
    resource: buildCloudBackupTargetResource(api),
    schema: CloudBackupTargetSchema,
  });
}
