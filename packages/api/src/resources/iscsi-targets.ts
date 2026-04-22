import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type IscsiTarget, IscsiTargetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildIscsiTargetResource(api: CustomObjectsApi): CrdResource<IscsiTarget> {
  return new CrdResource<IscsiTarget>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'iscsitargets' },
    schema: IscsiTargetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<IscsiTarget>({
    app,
    basePath: '/api/v1/iscsi-targets',
    tag: 'iscsi-targets',
    kind: 'IscsiTarget',
    resource: buildIscsiTargetResource(api),
    schema: IscsiTargetSchema,
  });
}
