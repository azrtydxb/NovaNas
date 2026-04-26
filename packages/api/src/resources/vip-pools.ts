import { type VipPool, VipPoolSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildVipPoolResource(db: DbClient): PgResource<VipPool> {
  return new PgResource<VipPool>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'VipPool',
    schema: VipPoolSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<VipPool>({
    app,
    basePath: '/api/v1/vip-pools',
    tag: 'vip-pools',
    kind: 'VipPool',
    resource: buildVipPoolResource(db),
    schema: VipPoolSchema,
  });
}
