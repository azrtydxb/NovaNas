import { type SmbServer, SmbServerSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildSmbServerResource(db: DbClient): PgResource<SmbServer> {
  return new PgResource<SmbServer>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'SmbServer',
    schema: SmbServerSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<SmbServer>({
    app,
    basePath: '/api/v1/smb-servers',
    tag: 'smb-servers',
    kind: 'SmbServer',
    resource: buildSmbServerResource(db),
    schema: SmbServerSchema,
  });
}
