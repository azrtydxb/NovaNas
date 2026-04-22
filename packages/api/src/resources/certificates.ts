import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Certificate, CertificateSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildCertificateResource(api: CustomObjectsApi): CrdResource<Certificate> {
  return new CrdResource<Certificate>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'certificates' },
    schema: CertificateSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Certificate>({
    app,
    basePath: '/api/v1/certificates',
    tag: 'certificates',
    kind: 'Certificate',
    resource: buildCertificateResource(api),
    schema: CertificateSchema,
  });
}
