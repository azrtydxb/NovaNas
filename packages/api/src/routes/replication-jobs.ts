import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { register as registerImpl } from '../resources/replication-jobs.js';
import { accepted, kubeErrorReply, nowIso, setAnnotation } from '../services/actions.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

const GVR = { group: 'novanas.io', version: 'v1alpha1', plural: 'replicationjobs' };

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function registerReplicationActions(app: FastifyInstance, api: CustomObjectsApi): void {
  const security = [{ sessionCookie: [] }];

  app.route<{ Params: { name: string } }>({
    method: 'POST',
    url: '/api/v1/replication-jobs/:name/run-now',
    preHandler: requireAuth,
    schema: { summary: 'Run a replication job immediately', tags: ['replication-jobs'], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'ReplicationJob', 'run-now')) return forbid(reply);
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
    url: '/api/v1/replication-jobs/:name/cancel',
    preHandler: requireAuth,
    schema: { summary: 'Cancel a running replication job', tags: ['replication-jobs'], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'ReplicationJob', 'cancel')) return forbid(reply);
      try {
        await setAnnotation(api, GVR, req.params.name, 'novanas.io/action-cancel', nowIso());
        return accepted({ message: `cancel requested for ${req.params.name}` });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });
}

export async function replicationJobsRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    registerReplicationActions(app, api);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/replication-jobs',
      summary: 'List replication jobs',
      tag: 'replication-jobs',
    },
    {
      method: 'POST',
      url: '/api/v1/replication-jobs',
      summary: 'Create a replication job',
      tag: 'replication-jobs',
    },
    {
      method: 'GET',
      url: '/api/v1/replication-jobs/:name',
      summary: 'Get a replication job',
      tag: 'replication-jobs',
    },
    {
      method: 'PATCH',
      url: '/api/v1/replication-jobs/:name',
      summary: 'Update a replication job',
      tag: 'replication-jobs',
    },
    {
      method: 'DELETE',
      url: '/api/v1/replication-jobs/:name',
      summary: 'Delete a replication job',
      tag: 'replication-jobs',
    },
  ]);
}
