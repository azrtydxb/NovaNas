import { type KmsKey, KmsKeySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildKmsKeyResource(db: DbClient): PgResource<KmsKey> {
  return new PgResource<KmsKey>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'KmsKey',
    schema: KmsKeySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<KmsKey>({
    app,
    basePath: '/api/v1/kms-keys',
    tag: 'kms-keys',
    kind: 'KmsKey',
    resource: buildKmsKeyResource(db),
    schema: KmsKeySchema,
  });
}
