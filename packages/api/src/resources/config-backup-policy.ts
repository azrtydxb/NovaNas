import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type ConfigBackupPolicy, ConfigBackupPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildConfigBackupPolicyResource(
  api: CustomObjectsApi
): CrdResource<ConfigBackupPolicy> {
  return new CrdResource<ConfigBackupPolicy>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'configbackuppolicies' },
    schema: ConfigBackupPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerSingletonRoutes<ConfigBackupPolicy>({
    app,
    basePath: '/api/v1/config-backup-policy',
    tag: 'config-backup-policy',
    kind: 'ConfigBackupPolicy',
    resource: buildConfigBackupPolicyResource(api),
    schema: ConfigBackupPolicySchema,
  });
}
