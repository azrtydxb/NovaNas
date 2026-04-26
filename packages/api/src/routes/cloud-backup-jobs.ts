import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import {
  buildCloudBackupJobResource,
  register as registerImpl,
} from '../resources/cloud-backup-jobs.js';
import { accepted, nowIso, setAnnotationOnResource } from '../services/actions.js';
import type { DbClient } from '../services/db.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function actionError(reply: FastifyReply, err: unknown) {
  if ((err as { name?: string })?.name === 'PgNotFoundError') {
    return reply.code(404).send({ error: 'not_found', message: (err as Error).message });
  }
  return reply
    .code(500)
    .send({ error: 'internal_error', message: (err as Error)?.message ?? String(err) });
}

function registerCloudBackupActions(app: FastifyInstance, db: DbClient): void {
  const security = [{ sessionCookie: [] }];
  const resource = buildCloudBackupJobResource(db);

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
    url: '/api/v1/cloud-backup-jobs/:name/cancel',
    preHandler: requireAuth,
    schema: { summary: 'Cancel a cloud backup job', tags: ['cloud-backup-jobs'], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'CloudBackupJob', 'cancel')) return forbid(reply);
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

export async function cloudBackupJobsRoutes(
  app: FastifyInstance,
  db?: DbClient | null
): Promise<void> {
  if (db) {
    registerImpl(app, db);
    registerCloudBackupActions(app, db);
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
