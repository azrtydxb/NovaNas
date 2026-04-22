import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/audit-policy.js';
import { registerStubs } from './_stubs.js';

export async function auditPolicyRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    return;
  }
  registerStubs(app, [
    {
      method: 'GET',
      url: '/api/v1/audit-policy',
      summary: 'Get audit policy',
      tag: 'audit-policy',
    },
    {
      method: 'PATCH',
      url: '/api/v1/audit-policy',
      summary: 'Update audit policy',
      tag: 'audit-policy',
    },
  ]);
}
