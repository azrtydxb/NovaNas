import { type BucketUser, BucketUserSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildBucketUserResource(db: DbClient): PgResource<BucketUser> {
  return new PgResource<BucketUser>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'BucketUser',
    schema: BucketUserSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<BucketUser>({
    app,
    basePath: '/api/v1/bucket-users',
    tag: 'bucket-users',
    kind: 'BucketUser',
    resource: buildBucketUserResource(db),
    schema: BucketUserSchema,
  });
}
