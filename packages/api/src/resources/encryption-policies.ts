import { type EncryptionPolicy, EncryptionPolicySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildEncryptionPolicyResource(db: DbClient): PgResource<EncryptionPolicy> {
  return new PgResource<EncryptionPolicy>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'EncryptionPolicy',
    schema: EncryptionPolicySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<EncryptionPolicy>({
    app,
    basePath: '/api/v1/encryption-policies',
    tag: 'encryption-policies',
    kind: 'EncryptionPolicy',
    resource: buildEncryptionPolicyResource(db),
    schema: EncryptionPolicySchema,
  });
}
