import { type TrafficPolicy, TrafficPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildTrafficPolicyResource(db: DbClient): PgResource<TrafficPolicy> {
  return new PgResource<TrafficPolicy>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'TrafficPolicy',
    schema: TrafficPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<TrafficPolicy>({
    app,
    basePath: '/api/v1/traffic-policies',
    tag: 'traffic-policies',
    kind: 'TrafficPolicy',
    resource: buildTrafficPolicyResource(db),
    schema: TrafficPolicySchema,
  });
}
