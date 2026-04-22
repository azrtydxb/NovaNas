import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type BucketUser, BucketUserSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildBucketUserResource(api: CustomObjectsApi): CrdResource<BucketUser> {
  return new CrdResource<BucketUser>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'bucketusers' },
    schema: BucketUserSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<BucketUser>({
    app,
    basePath: '/api/v1/bucket-users',
    tag: 'bucket-users',
    kind: 'BucketUser',
    resource: buildBucketUserResource(api),
    schema: BucketUserSchema,
  });
}
