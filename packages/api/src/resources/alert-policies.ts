import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type AlertPolicy, AlertPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildAlertPolicyResource(api: CustomObjectsApi): CrdResource<AlertPolicy> {
  return new CrdResource<AlertPolicy>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'alertpolicies' },
    schema: AlertPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<AlertPolicy>({
    app,
    basePath: '/api/v1/alert-policies',
    tag: 'alert-policies',
    kind: 'AlertPolicy',
    resource: buildAlertPolicyResource(api),
    schema: AlertPolicySchema,
  });
}
