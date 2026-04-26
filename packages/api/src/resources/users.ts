import { type User, UserSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildUserResource(db: DbClient): PgResource<User> {
  return new PgResource<User>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'User',
    schema: UserSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<User>({
    app,
    basePath: '/api/v1/users',
    tag: 'users',
    kind: 'User',
    resource: buildUserResource(db),
    schema: UserSchema,
  });
}
