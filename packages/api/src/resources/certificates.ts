import { type Certificate, CertificateSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { registerCrudRoutes } from './_register.js';

export function buildCertificateResource(db: DbClient): PgResource<Certificate> {
  return new PgResource<Certificate>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'Certificate',
    schema: CertificateSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, db: DbClient): void {
  registerCrudRoutes<Certificate>({
    app,
    basePath: '/api/v1/certificates',
    tag: 'certificates',
    kind: 'Certificate',
    resource: buildCertificateResource(db),
    schema: CertificateSchema,
  });
}
