import { type CustomDomain, CustomDomainSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildCustomDomainResource(db: DbClient): PgResource<CustomDomain> {
  return new PgResource<CustomDomain>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'CustomDomain',
    schema: CustomDomainSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<CustomDomain>({
    app,
    basePath: '/api/v1/custom-domains',
    tag: 'custom-domains',
    kind: 'CustomDomain',
    resource: buildCustomDomainResource(db),
    schema: CustomDomainSchema,
  });
}
