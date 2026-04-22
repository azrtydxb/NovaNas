import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type SystemSettings, SystemSettingsSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildSystemSettingsResource(api: CustomObjectsApi): CrdResource<SystemSettings> {
  return new CrdResource<SystemSettings>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'systemsettings' },
    schema: SystemSettingsSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerSingletonRoutes<SystemSettings>({
    app,
    basePath: '/api/v1/system/settings',
    tag: 'system-settings',
    kind: 'SystemSettings',
    resource: buildSystemSettingsResource(api),
    schema: SystemSettingsSchema,
  });
}
