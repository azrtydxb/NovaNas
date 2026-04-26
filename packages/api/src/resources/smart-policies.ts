import { type SmartPolicy, SmartPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildSmartPolicyResource(db: DbClient): PgResource<SmartPolicy> {
  return new PgResource<SmartPolicy>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'SmartPolicy',
    schema: SmartPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<SmartPolicy>({
    app,
    basePath: '/api/v1/smart-policies',
    tag: 'smart-policies',
    kind: 'SmartPolicy',
    resource: buildSmartPolicyResource(db),
    schema: SmartPolicySchema,
  });
}
