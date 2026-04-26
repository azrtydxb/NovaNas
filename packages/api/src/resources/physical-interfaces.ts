import { type PhysicalInterface, PhysicalInterfaceSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerReadOnlyRoutes } from './_register_extras.js';

export function buildPhysicalInterfaceResource(db: DbClient): PgResource<PhysicalInterface> {
  return new PgResource<PhysicalInterface>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'PhysicalInterface',
    schema: PhysicalInterfaceSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  // PhysicalInterface is observed, not authored — the host network
  // agent posts what it discovered, the SPA reads it. POST/PATCH/
  // DELETE return 405.
  registerReadOnlyRoutes<PhysicalInterface>({
    app,
    basePath: '/api/v1/physical-interfaces',
    tag: 'physical-interfaces',
    kind: 'PhysicalInterface',
    resource: buildPhysicalInterfaceResource(db),
    schema: PhysicalInterfaceSchema,
  });
}
