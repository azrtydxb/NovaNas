import type { DbClient } from '../services/db.js';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import {
  buildReplicationJobResource,
  register as registerImpl,
} from '../resources/replication-jobs.js';
import { accepted, nowIso, setAnnotationOnResource } from '../services/actions.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

const GVR = { group: 'novanas.io', version: 'v1alpha1', plural: 'replicationjobs' };

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function registerReplicationActions(app: FastifyInstance, db: DbClient): void {
  const security = [{ sessionCookie: [] }];
  const resource = buildReplicationJobResource(db);

  function actionError(reply: FastifyReply, err: unknown) {
    if ((err as { name?: string })?.name === 'PgNotFoundError') {
      return reply.code(404).send({ error: 'not_found', message: (err as Error).message });
    }
    return reply.code(500).send({ error: 'internal_error', message: (err as Error).message });
  }

  app.route<{ Params: { name: string } }>({
    method: 'POST',
    url: '/api/v1/replication-jobs/:name/run-now',
    preHandler: requireAuth,
    schema: { summary: 'Run a replication job immediately', tags: ['replication-jobs'], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'ReplicationJob', 'run-now')) return forbid(reply);
      try {
        await setAnnotationOnResource(
          resource,
          req.params.name,
          'novanas.io/action-run-now',
          nowIso()
        );
        return accepted({ message: `run-now requested for ${req.params.name}` });
      } catch (err) {
        return actionError(reply, err);
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
        await setAnnotationOnResource(
          resource,
          req.params.name,
          'novanas.io/action-cancel',
          nowIso()
        );
        return accepted({ message: `cancel requested for ${req.params.name}` });
      } catch (err) {
        return actionError(reply, err);
      }
    },
  });
}

export async function replicationJobsRoutes(
  app: FastifyInstance,
  db?: DbClient | null
): Promise<void> {
  if (db) {
    registerImpl(app, db);
    registerReplicationActions(app, db);
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
