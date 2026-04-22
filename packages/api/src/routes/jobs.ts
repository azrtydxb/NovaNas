import type { FastifyInstance } from 'fastify';
import { z } from 'zod';
import { AuthzRole } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import type { JobState, JobsService } from '../services/jobs.js';

export interface JobsRouteDeps {
  jobs: JobsService | null;
}

const VALID_STATES: readonly JobState[] = ['queued', 'running', 'succeeded', 'failed', 'cancelled'];

const listQuery = z.object({
  state: z.enum(['queued', 'running', 'succeeded', 'failed', 'cancelled']).optional(),
  kind: z.string().max(128).optional(),
  ownerId: z.string().uuid().optional(),
  limit: z.coerce.number().int().positive().max(500).optional(),
});

function isAdmin(roles: string[]): boolean {
  return roles.includes(AuthzRole.Admin);
}

export async function jobsRoutes(app: FastifyInstance, deps: JobsRouteDeps): Promise<void> {
  const svc = deps.jobs;

  app.get(
    '/api/v1/jobs',
    {
      preHandler: requireAuth,
      schema: { tags: ['jobs'], summary: 'List background jobs' },
    },
    async (req, reply) => {
      if (!svc) return reply.code(503).send({ error: 'db_unavailable' });
      const parsed = listQuery.safeParse(req.query);
      if (!parsed.success) {
        return reply.code(400).send({ error: 'invalid_query', details: parsed.error.format() });
      }
      const user = req.user!;
      const ownerId = isAdmin(user.roles) ? parsed.data.ownerId : user.sub;
      const items = await svc.list({
        state: parsed.data.state,
        kind: parsed.data.kind,
        ownerId,
        limit: parsed.data.limit,
      });
      return { items };
    }
  );

  app.get(
    '/api/v1/jobs/:id',
    {
      preHandler: requireAuth,
      schema: { tags: ['jobs'], summary: 'Get a job by id' },
    },
    async (req, reply) => {
      if (!svc) return reply.code(503).send({ error: 'db_unavailable' });
      const { id } = req.params as { id: string };
      const job = await svc.get(id);
      if (!job) return reply.code(404).send({ error: 'not_found' });
      const user = req.user!;
      if (!isAdmin(user.roles) && job.ownerId && job.ownerId !== user.sub) {
        return reply.code(403).send({ error: 'forbidden' });
      }
      return job;
    }
  );

  app.delete(
    '/api/v1/jobs/:id',
    {
      preHandler: requireAuth,
      schema: { tags: ['jobs'], summary: 'Cancel a job' },
    },
    async (req, reply) => {
      if (!svc) return reply.code(503).send({ error: 'db_unavailable' });
      const { id } = req.params as { id: string };
      const existing = await svc.get(id);
      if (!existing) return reply.code(404).send({ error: 'not_found' });
      const user = req.user!;
      if (!isAdmin(user.roles) && existing.ownerId && existing.ownerId !== user.sub) {
        return reply.code(403).send({ error: 'forbidden' });
      }
      const row = await svc.cancel(id);
      return { ok: true, job: row };
    }
  );

  // Expose the valid states for UI dropdowns.
  app.get(
    '/api/v1/jobs/_meta/states',
    { preHandler: requireAuth, schema: { tags: ['jobs'] } },
    async () => ({ states: VALID_STATES })
  );
}
