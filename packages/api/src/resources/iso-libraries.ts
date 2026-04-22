import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type IsoLibrary, IsoLibrarySchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildIsoLibraryResource(api: CustomObjectsApi): CrdResource<IsoLibrary> {
  return new CrdResource<IsoLibrary>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'isolibraries' },
    schema: IsoLibrarySchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<IsoLibrary>({
    app,
    basePath: '/api/v1/iso-libraries',
    tag: 'iso-libraries',
    kind: 'IsoLibrary',
    resource: buildIsoLibraryResource(api),
    schema: IsoLibrarySchema,
  });
}
