import { cpus, hostname, loadavg, networkInterfaces, totalmem, uptime } from 'node:os';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { accepted, triggerJob } from '../services/actions.js';
import type { JobsService } from '../services/jobs.js';
import type { AuthenticatedUser } from '../types.js';

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

/**
 * System routes.
 *
 * `/system/info` and `/system/network` read live OS data from the control-plane
 * pod. `/system/alerts` and `/system/events` proxy an empty list until the
 * alert/event aggregators land; they return a properly typed response body
 * (rather than a stub envelope) so clients can consume them today.
 *
 * `/system/reboot`, `/system/shutdown`, `/system/reset`, `/system/support-bundle`
 * and `/system/check-update` enqueue Jobs that the system controller drains.
 */
export async function systemRoutes(
  app: FastifyInstance,
  deps: SystemRoutesDeps = {}
): Promise<void> {
  const security = [{ sessionCookie: [] }];
  const { jobs } = deps;

  // GET /api/v1/system/info
  app.route({
    method: 'GET',
    url: '/api/v1/system/info',
    preHandler: requireAuth,
    schema: {
      summary: 'System info (CPU, RAM, uptime)',
      tags: ['system'],
      security,
      response: {
        200: {
          type: 'object',
          properties: {
            hostname: { type: 'string' },
            cpuCount: { type: 'number' },
            cpuModel: { type: 'string' },
            totalMemoryBytes: { type: 'number' },
            uptimeSeconds: { type: 'number' },
            loadAvg: { type: 'array', items: { type: 'number' } },
          },
          required: [
            'hostname',
            'cpuCount',
            'cpuModel',
            'totalMemoryBytes',
            'uptimeSeconds',
            'loadAvg',
          ],
        },
      },
    },
    handler: async () => {
      const cpuList = cpus();
      return {
        hostname: hostname(),
        cpuCount: cpuList.length,
        cpuModel: cpuList[0]?.model ?? 'unknown',
        totalMemoryBytes: totalmem(),
        uptimeSeconds: Math.floor(uptime()),
        loadAvg: loadavg(),
      };
    },
  });

  // GET /api/v1/system/network
  app.route({
    method: 'GET',
    url: '/api/v1/system/network',
    preHandler: requireAuth,
    schema: {
      summary: 'Network interfaces visible to the control-plane pod',
      tags: ['system'],
      security,
    },
    handler: async () => {
      const nics = networkInterfaces();
      const interfaces = Object.entries(nics).map(([name, addrs]) => ({
        name,
        addresses: (addrs ?? []).map((a) => ({
          family: a.family,
          address: a.address,
          netmask: a.netmask,
          mac: a.mac,
          internal: a.internal,
          cidr: a.cidr,
        })),
      }));
      return { interfaces };
    },
  });

  // GET /api/v1/system/alerts
  app.route({
    method: 'GET',
    url: '/api/v1/system/alerts',
    preHandler: requireAuth,
    schema: {
      summary: 'Active alerts',
      tags: ['system'],
      security,
      response: {
        200: {
          type: 'object',
          properties: { items: { type: 'array', items: { type: 'object' } } },
          required: ['items'],
        },
      },
    },
    // Alert aggregation is provided by the alert-manager reconciler (see
    // alert-policies / alert-channels routes). Until a dedicated roll-up is
    // surfaced here we return an empty list with a stable shape.
    handler: async () => ({ items: [] as unknown[] }),
  });

  // GET /api/v1/system/events
  app.route({
    method: 'GET',
    url: '/api/v1/system/events',
    preHandler: requireAuth,
    schema: {
      summary: 'Recent events',
      tags: ['system'],
      security,
      response: {
        200: {
          type: 'object',
          properties: { items: { type: 'array', items: { type: 'object' } } },
          required: ['items'],
        },
      },
    },
    handler: async () => ({ items: [] as unknown[] }),
  });

  // POST /api/v1/system/reboot
  app.route({
    method: 'POST',
    url: '/api/v1/system/reboot',
    preHandler: requireAuth,
    schema: { summary: 'Reboot the appliance', tags: ['system'], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'SystemSettings', 'reboot')) return forbid(reply);
      if (!requireJobs(reply, jobs)) return reply;
      const jobId = await triggerJob(jobs, 'system.reboot', {}, user.sub);
      return accepted({ jobId, status: 'pending', message: 'reboot queued' });
    },
  });

  // POST /api/v1/system/shutdown
  app.route({
    method: 'POST',
    url: '/api/v1/system/shutdown',
    preHandler: requireAuth,
    schema: { summary: 'Shutdown the appliance', tags: ['system'], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'SystemSettings', 'shutdown')) return forbid(reply);
      if (!requireJobs(reply, jobs)) return reply;
      const jobId = await triggerJob(jobs, 'system.shutdown', {}, user.sub);
      return accepted({ jobId, status: 'pending', message: 'shutdown queued' });
    },
  });

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
