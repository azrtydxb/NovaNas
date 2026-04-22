import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { accepted, triggerJob } from '../services/actions.js';
import type { JobsService } from '../services/jobs.js';
import type { AuthenticatedUser } from '../types.js';
import { registerStubs } from './_stubs.js';

export interface SystemRoutesDeps {
  jobs?: JobsService | null;
}

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function requireJobs(reply: FastifyReply, jobs?: JobsService | null): jobs is JobsService {
  if (!jobs) {
    reply.code(503).send({ error: 'db_unavailable', message: 'jobs service not available' });
    return false;
  }
  return true;
}

export async function systemRoutes(
  app: FastifyInstance,
  deps: SystemRoutesDeps = {}
): Promise<void> {
  const security = [{ sessionCookie: [] }];
  const { jobs } = deps;

  registerStubs(app, [
    {
      method: 'GET',
      url: '/api/v1/system/info',
      summary: 'System info (CPU, RAM, uptime)',
      tag: 'system',
    },
    { method: 'GET', url: '/api/v1/system/network', summary: 'Network interfaces', tag: 'system' },
    { method: 'GET', url: '/api/v1/system/alerts', summary: 'Active alerts', tag: 'system' },
    { method: 'GET', url: '/api/v1/system/events', summary: 'Recent events', tag: 'system' },
    {
      method: 'POST',
      url: '/api/v1/system/reboot',
      summary: 'Reboot the appliance',
      tag: 'system',
    },
    {
      method: 'POST',
      url: '/api/v1/system/shutdown',
      summary: 'Shutdown the appliance',
      tag: 'system',
    },
  ]);

  // POST /api/v1/system/reset?tier=soft|config|full
  app.route<{ Querystring: { tier?: string } }>({
    method: 'POST',
    url: '/api/v1/system/reset',
    preHandler: requireAuth,
    schema: {
      summary: 'Factory reset (soft/config/full). Returns a job id.',
      tags: ['system'],
      security,
      querystring: {
        type: 'object',
        properties: { tier: { type: 'string', enum: ['soft', 'config', 'full'] } },
      },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'SystemSettings', 'reset')) return forbid(reply);
      const tier = req.query.tier ?? 'soft';
      if (!['soft', 'config', 'full'].includes(tier)) {
        return reply
          .code(400)
          .send({ error: 'invalid_query', message: 'tier must be soft|config|full' });
      }
      if (!requireJobs(reply, jobs)) return reply;
      const jobId = await triggerJob(jobs, 'system.reset', { tier }, user.sub);
      const warnings =
        tier === 'full' ? ['full reset erases all data and configuration'] : undefined;
      return accepted({
        jobId,
        status: 'pending',
        message: `system reset (${tier}) queued`,
        warnings,
      });
    },
  });

  // POST /api/v1/system/support-bundle
  app.route({
    method: 'POST',
    url: '/api/v1/system/support-bundle',
    preHandler: requireAuth,
    schema: {
      summary: 'Generate a support bundle. Returns job id + download URL.',
      tags: ['system'],
      security,
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'SystemSettings', 'support-bundle')) return forbid(reply);
      if (!requireJobs(reply, jobs)) return reply;
      const jobId = await triggerJob(jobs, 'system.supportBundle', {}, user.sub);
      return {
        ...accepted({ jobId, status: 'pending', message: 'support bundle generation queued' }),
        downloadUrl: `/api/v1/jobs/${jobId}/artifact`,
      };
    },
  });

  // POST /api/v1/system/check-update
  app.route({
    method: 'POST',
    url: '/api/v1/system/check-update',
    preHandler: requireAuth,
    schema: {
      summary: 'Check for system updates. Returns job id.',
      tags: ['system'],
      security,
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'SystemSettings', 'check-update')) return forbid(reply);
      if (!requireJobs(reply, jobs)) return reply;
      const jobId = await triggerJob(jobs, 'system.checkUpdate', {}, user.sub);
      return accepted({ jobId, status: 'pending', message: 'update check queued' });
    },
  });
}
