import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type AuditPolicy, AuditPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerSingletonRoutes } from './_register_extras.js';

export function buildAuditPolicyResource(api: CustomObjectsApi): CrdResource<AuditPolicy> {
  return new CrdResource<AuditPolicy>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'auditpolicies' },
    schema: AuditPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerSingletonRoutes<AuditPolicy>({
    app,
    basePath: '/api/v1/audit-policy',
    tag: 'audit-policy',
    kind: 'AuditPolicy',
    resource: buildAuditPolicyResource(api),
    schema: AuditPolicySchema,
  });
}
