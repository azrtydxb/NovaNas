import { type HostInterface, HostInterfaceSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildHostInterfaceResource(db: DbClient): PgResource<HostInterface> {
  return new PgResource<HostInterface>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'HostInterface',
    schema: HostInterfaceSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<HostInterface>({
    app,
    basePath: '/api/v1/host-interfaces',
    tag: 'host-interfaces',
    kind: 'HostInterface',
    resource: buildHostInterfaceResource(db),
    schema: HostInterfaceSchema,
  });
}
