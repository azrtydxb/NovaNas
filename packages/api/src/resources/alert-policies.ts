import { type AlertPolicy, AlertPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildAlertPolicyResource(db: DbClient): PgResource<AlertPolicy> {
  return new PgResource<AlertPolicy>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'AlertPolicy',
    schema: AlertPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<AlertPolicy>({
    app,
    basePath: '/api/v1/alert-policies',
    tag: 'alert-policies',
    kind: 'AlertPolicy',
    resource: buildAlertPolicyResource(db),
    schema: AlertPolicySchema,
  });
}
