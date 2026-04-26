import { type SystemSettings, SystemSettingsSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildSystemSettingsResource(db: DbClient): PgResource<SystemSettings> {
  return new PgResource<SystemSettings>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'SystemSettings',
    schema: SystemSettingsSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerSingletonRoutes<SystemSettings>({
    app,
    basePath: '/api/v1/system/settings',
    tag: 'system-settings',
    kind: 'SystemSettings',
    resource: buildSystemSettingsResource(db),
    schema: SystemSettingsSchema,
  });
}
