import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type Dataset, DatasetSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildDatasetResource(api: CustomObjectsApi): CrdResource<Dataset> {
  return new CrdResource<Dataset>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'datasets' },
    schema: DatasetSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<Dataset>({
    app,
    basePath: '/api/v1/datasets',
    tag: 'datasets',
    kind: 'Dataset',
    resource: buildDatasetResource(api),
    schema: DatasetSchema,
  });
}
