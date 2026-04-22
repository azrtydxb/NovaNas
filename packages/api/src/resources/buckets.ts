import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Bucket, BucketSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildBucketResource(api: CustomObjectsApi): CrdResource<Bucket> {
  return new CrdResource<Bucket>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'buckets' },
    schema: BucketSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Bucket>({
    app,
    basePath: '/api/v1/buckets',
    tag: 'buckets',
    kind: 'Bucket',
    resource: buildBucketResource(api),
    schema: BucketSchema,
  });
}
