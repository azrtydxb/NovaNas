import { type ServicePolicy, ServicePolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildServicePolicyResource(db: DbClient): PgResource<ServicePolicy> {
  return new PgResource<ServicePolicy>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'ServicePolicy',
    schema: ServicePolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerSingletonRoutes<ServicePolicy>({
    app,
    basePath: '/api/v1/service-policy',
    tag: 'service-policy',
    kind: 'ServicePolicy',
    resource: buildServicePolicyResource(db),
    schema: ServicePolicySchema,
  });
}
