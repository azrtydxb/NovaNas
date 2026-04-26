import { type Bucket, BucketSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildBucketResource(db: DbClient): PgResource<Bucket> {
  return new PgResource<Bucket>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Bucket',
    schema: BucketSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Bucket>({
    app,
    basePath: '/api/v1/buckets',
    tag: 'buckets',
    kind: 'Bucket',
    resource: buildBucketResource(db),
    schema: BucketSchema,
  });
}
