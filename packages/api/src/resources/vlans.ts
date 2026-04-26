import { type Vlan, VlanSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildVlanResource(db: DbClient): PgResource<Vlan> {
  return new PgResource<Vlan>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Vlan',
    schema: VlanSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Vlan>({
    app,
    basePath: '/api/v1/vlans',
    tag: 'vlans',
    kind: 'Vlan',
    resource: buildVlanResource(db),
    schema: VlanSchema,
  });
}
