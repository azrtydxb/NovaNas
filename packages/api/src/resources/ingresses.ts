import { type Ingress, IngressSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildIngressResource(db: DbClient): PgResource<Ingress> {
  return new PgResource<Ingress>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Ingress',
    schema: IngressSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Ingress>({
    app,
    basePath: '/api/v1/ingresses',
    tag: 'ingresses',
    kind: 'Ingress',
    resource: buildIngressResource(db),
    schema: IngressSchema,
  });
}
