import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type AlertChannel, AlertChannelSchema } from '@novanas/schemas';
import type { FastifyInstance } from 'fastify';
import { CrdResource } from '../services/crd.js';
import { registerCrudRoutes } from './_register.js';

export function buildAlertChannelResource(api: CustomObjectsApi): CrdResource<AlertChannel> {
  return new CrdResource<AlertChannel>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'alertchannels' },
    schema: AlertChannelSchema,
    namespaced: false,
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  registerCrudRoutes<AlertChannel>({
    app,
    basePath: '/api/v1/alert-channels',
    tag: 'alert-channels',
    kind: 'AlertChannel',
    resource: buildAlertChannelResource(api),
    schema: AlertChannelSchema,
  });
}
