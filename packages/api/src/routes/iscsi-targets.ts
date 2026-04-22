import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { register as registerIscsiTargets } from '../resources/iscsi-targets.js';
import { registerStubs } from './_stubs.js';

export async function iscsiTargetRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerIscsiTargets(app, api);
    return;
  }
  registerStubs(app, [
    {
      method: 'GET',
      url: '/api/v1/iscsi-targets',
      summary: 'List iSCSI targets',
      tag: 'iscsi-targets',
    },
    {
      method: 'POST',
      url: '/api/v1/iscsi-targets',
      summary: 'Create an iSCSI target',
      tag: 'iscsi-targets',
    },
    {
      method: 'GET',
      url: '/api/v1/iscsi-targets/:name',
      summary: 'Get an iSCSI target',
      tag: 'iscsi-targets',
    },
    {
      method: 'PATCH',
      url: '/api/v1/iscsi-targets/:name',
      summary: 'Update an iSCSI target',
      tag: 'iscsi-targets',
    },
    {
      method: 'DELETE',
      url: '/api/v1/iscsi-targets/:name',
      summary: 'Delete an iSCSI target',
      tag: 'iscsi-targets',
    },
  ]);
}
