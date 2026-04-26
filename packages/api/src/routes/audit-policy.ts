import type { FastifyInstance } from 'fastify';
import { register as registerImpl } from '../resources/audit-policy.js';
import type { DbClient } from '../services/db.js';
import { registerUnavailable } from './_unavailable.js';

export async function auditPolicyRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/audit-policy',
      summary: 'List AuditPolicys',
      tag: 'audit-policy',
    },
    {
      method: 'POST',
      url: '/api/v1/audit-policy',
      summary: 'Create a AuditPolicy',
      tag: 'audit-policy',
    },
    {
      method: 'GET',
      url: '/api/v1/audit-policy/:name',
      summary: 'Get a AuditPolicy',
      tag: 'audit-policy',
    },
    {
      method: 'PATCH',
      url: '/api/v1/audit-policy/:name',
      summary: 'Update a AuditPolicy',
      tag: 'audit-policy',
    },
    {
      method: 'DELETE',
      url: '/api/v1/audit-policy/:name',
      summary: 'Delete a AuditPolicy',
      tag: 'audit-policy',
    },
  ]);
}
