import { type Bond, BondSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildBondResource(db: DbClient): PgResource<Bond> {
  return new PgResource<Bond>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Bond',
    schema: BondSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Bond>({
    app,
    basePath: '/api/v1/bonds',
    tag: 'bonds',
    kind: 'Bond',
    resource: buildBondResource(db),
    schema: BondSchema,
  });
}
