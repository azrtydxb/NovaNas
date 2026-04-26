import { type Certificate, CertificateSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import type { CertManagerClient } from '../services/cert-manager.js';
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

export function register(
  app: FastifyInstance,
  db: DbClient,
  certManager?: CertManagerClient | null,
  systemNamespace = 'novanas-system'
): void {
  registerCrudRoutes<Certificate>({
    app,
    basePath: '/api/v1/certificates',
    tag: 'certificates',
    kind: 'Certificate',
    resource: buildCertificateResource(db),
    schema: CertificateSchema,
    // Project NovaNas Certificate onto a cert-manager Certificate CR.
    // This is the one place the api still WRITES kube objects post-
    // CRD-migration (#51) — cert-manager IS the engine here.
    afterCreate: certManager
      ? async (cert, req) => {
          await certManager.ensureCertificate(cert, systemNamespace);
          req.log.debug(
            { kind: 'Certificate', name: cert.metadata.name },
            'cert-manager Certificate ensured'
          );
        }
      : undefined,
    afterPatch: certManager
      ? async (cert, _patch, req) => {
          await certManager.ensureCertificate(cert, systemNamespace);
          req.log.debug(
            { kind: 'Certificate', name: cert.metadata.name },
            'cert-manager Certificate updated'
          );
        }
      : undefined,
    afterDelete: certManager
      ? async (name, req) => {
          await certManager.deleteCertificate(name, systemNamespace);
          req.log.debug({ kind: 'Certificate', name }, 'cert-manager Certificate deleted');
        }
      : undefined,
  });
}
