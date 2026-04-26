import { type Vm, VmSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes, userNamespaceResolver } from './_register.js';

export function buildVmResource(db: DbClient): PgResource<Vm> {
  return new PgResource<Vm>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Vm',
    schema: VmSchema,
    namespaced: true,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Vm>({
    app,
    basePath: '/api/v1/vms',
    tag: 'vms',
    kind: 'Vm',
    resource: buildVmResource(db),
    schema: VmSchema,
    resolveNamespace: userNamespaceResolver,
  });
}
