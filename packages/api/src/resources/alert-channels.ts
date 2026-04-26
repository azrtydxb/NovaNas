import { type AlertChannel, AlertChannelSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildAlertChannelResource(db: DbClient): PgResource<AlertChannel> {
  return new PgResource<AlertChannel>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'AlertChannel',
    schema: AlertChannelSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<AlertChannel>({
    app,
    basePath: '/api/v1/alert-channels',
    tag: 'alert-channels',
    kind: 'AlertChannel',
    resource: buildAlertChannelResource(db),
    schema: AlertChannelSchema,
  });
}
