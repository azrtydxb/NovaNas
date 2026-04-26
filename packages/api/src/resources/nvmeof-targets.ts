import { type NvmeofTarget, NvmeofTargetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildNvmeofTargetResource(db: DbClient): PgResource<NvmeofTarget> {
  return new PgResource<NvmeofTarget>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'NvmeofTarget',
    schema: NvmeofTargetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<NvmeofTarget>({
    app,
    basePath: '/api/v1/nvmeof-targets',
    tag: 'nvmeof-targets',
    kind: 'NvmeofTarget',
    resource: buildNvmeofTargetResource(db),
    schema: NvmeofTargetSchema,
  });
}
