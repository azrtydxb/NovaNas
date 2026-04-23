import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { register as registerImpl } from '../resources/cloud-backup-jobs.js';
import { accepted, kubeErrorReply, nowIso, setAnnotation } from '../services/actions.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

const GVR = { group: 'novanas.io', version: 'v1alpha1', plural: 'cloudbackupjobs' };

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function registerCloudBackupActions(app: FastifyInstance, api: CustomObjectsApi): void {
  const security = [{ sessionCookie: [] }];

  app.route<{ Params: { name: string } }>({
    method: 'POST',
    url: '/api/v1/cloud-backup-jobs/:name/run-now',
    preHandler: requireAuth,
    schema: {
      summary: 'Run a cloud backup job immediately',
      tags: ['cloud-backup-jobs'],
      security,
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'CloudBackupJob', 'run-now')) return forbid(reply);
      try {
        await setAnnotation(api, GVR, req.params.name, 'novanas.io/action-run-now', nowIso());
        return accepted({ message: `run-now requested for ${req.params.name}` });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });

  app.route<{ Params: { name: string } }>({
    method: 'POST',
    url: '/api/v1/cloud-backup-jobs/:name/cancel',
    preHandler: requireAuth,
    schema: { summary: 'Cancel a cloud backup job', tags: ['cloud-backup-jobs'], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'CloudBackupJob', 'cancel')) return forbid(reply);
      try {
        await setAnnotation(api, GVR, req.params.name, 'novanas.io/action-cancel', nowIso());
        return accepted({ message: `cancel requested for ${req.params.name}` });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });
}

export async function cloudBackupJobsRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    registerCloudBackupActions(app, api);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/cloud-backup-jobs',
      summary: 'List cloud backup jobs',
      tag: 'cloud-backup-jobs',
    },
    {
      method: 'POST',
      url: '/api/v1/cloud-backup-jobs',
      summary: 'Create a cloud backup job',
      tag: 'cloud-backup-jobs',
    },
    {
      method: 'GET',
      url: '/api/v1/cloud-backup-jobs/:name',
      summary: 'Get a cloud backup job',
      tag: 'cloud-backup-jobs',
    },
    {
      method: 'PATCH',
      url: '/api/v1/cloud-backup-jobs/:name',
      summary: 'Update a cloud backup job',
      tag: 'cloud-backup-jobs',
    },
    {
      method: 'DELETE',
      url: '/api/v1/cloud-backup-jobs/:name',
      summary: 'Delete a cloud backup job',
      tag: 'cloud-backup-jobs',
    },
  ]);
}
