import { type AuditPolicy, AuditPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildAuditPolicyResource(db: DbClient): PgResource<AuditPolicy> {
  return new PgResource<AuditPolicy>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'AuditPolicy',
    schema: AuditPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerSingletonRoutes<AuditPolicy>({
    app,
    basePath: '/api/v1/audit-policy',
    tag: 'audit-policy',
    kind: 'AuditPolicy',
    resource: buildAuditPolicyResource(db),
    schema: AuditPolicySchema,
  });
}
