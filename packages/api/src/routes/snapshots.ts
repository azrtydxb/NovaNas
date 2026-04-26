import type { DbClient } from '../services/db.js';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { register as registerSnapshots } from '../resources/snapshots.js';
import { accepted, triggerJob } from '../services/actions.js';
import type { JobsService } from '../services/jobs.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

export interface SnapshotRoutesDeps {
  db?: DbClient | null;
  jobs?: JobsService | null;
}

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function registerSnapshotActions(app: FastifyInstance, jobs: JobsService | null): void {
  const security = [{ sessionCookie: [] }];

  // POST /api/v1/snapshots/:name/restore — body: { targetVolume }
  app.route<{
    Params: { name: string };
    Body: { targetVolume?: string };
  }>({
    method: 'POST',
    url: '/api/v1/snapshots/:name/restore',
    preHandler: requireAuth,
    schema: {
      summary: 'Restore a snapshot into a target volume. Returns job id.',
      tags: ['snapshots'],
      security,
      body: {
        type: 'object',
        properties: { targetVolume: { type: 'string' } },
        required: ['targetVolume'],
      },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'Snapshot', 'restore')) return forbid(reply);
      const targetVolume = req.body?.targetVolume;
      if (!targetVolume) {
        return reply.code(400).send({ error: 'invalid_body', message: 'targetVolume required' });
      }
      if (!jobs) {
        return reply
          .code(503)
          .send({ error: 'db_unavailable', message: 'jobs service not available' });
      }
      // Operator-side TODO: the snapshot controller watches jobs of kind
      // `snapshot.restore` and performs the actual volume provisioning +
      // data restore.
      const jobId = await triggerJob(
        jobs,
        'snapshot.restore',
        { snapshot: req.params.name, targetVolume },
        user.sub
      );
      return accepted({
        jobId,
        status: 'pending',
        message: `restore of ${req.params.name} queued`,
      });
    },
  });
}

export async function snapshotRoutes(
  app: FastifyInstance,
  db?: DbClient | null,
  deps: { jobs?: JobsService | null } = {}
): Promise<void> {
  if (db) {
    registerSnapshots(app, db);
    registerSnapshotActions(app, deps.jobs ?? null);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/snapshots', summary: 'List snapshots', tag: 'snapshots' },
    { method: 'POST', url: '/api/v1/snapshots', summary: 'Take a snapshot', tag: 'snapshots' },
    {
      method: 'GET',
      url: '/api/v1/snapshots/:name',
      summary: 'Get a snapshot',
      tag: 'snapshots',
    },
    {
      method: 'PATCH',
      url: '/api/v1/snapshots/:name',
      summary: 'Update a snapshot',
      tag: 'snapshots',
    },
    {
      method: 'DELETE',
      url: '/api/v1/snapshots/:name',
      summary: 'Delete a snapshot',
      tag: 'snapshots',
    },
  ]);
}
