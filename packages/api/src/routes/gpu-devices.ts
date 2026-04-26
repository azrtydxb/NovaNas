import type { GpuDevice } from '@novanas/schemas';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { buildGpuDeviceResource, register as registerImpl } from '../resources/gpu-devices.js';
import { accepted } from '../services/actions.js';
import type { DbClient } from '../services/db.js';
import type { PgResource } from '../services/pg-resource.js';
import { isNotFound } from '../services/resource.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function notFound(reply: FastifyReply, name: string): FastifyReply {
  return reply.code(404).send({ error: 'not_found', message: `gpu '${name}' not found` });
}

function registerGpuActions(app: FastifyInstance, resource: PgResource<GpuDevice>): void {
  const security = [{ sessionCookie: [] }];

  app.route<{
    Params: { name: string };
    Body: { vmNamespace?: string; vmName?: string };
  }>({
    method: 'POST',
    url: '/api/v1/gpu-devices/:name/assign',
    preHandler: requireAuth,
    schema: {
      summary: 'Assign a GPU device to a VM',
      tags: ['gpu-devices'],
      security,
      body: {
        type: 'object',
        properties: {
          vmNamespace: { type: 'string' },
          vmName: { type: 'string' },
        },
        required: ['vmNamespace', 'vmName'],
      },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'GpuDevice', 'assign')) return forbid(reply);
      const { vmNamespace, vmName } = req.body ?? {};
      if (!vmNamespace || !vmName) {
        return reply
          .code(400)
          .send({ error: 'invalid_body', message: 'vmNamespace and vmName required' });
      }
      try {
        await resource.patch(req.params.name, {
          spec: { assignedTo: { namespace: vmNamespace, name: vmName } },
        });
        return accepted({ message: `assigned to ${vmNamespace}/${vmName}` });
      } catch (err) {
        if (isNotFound(err)) return notFound(reply, req.params.name);
        throw err;
      }
    },
  });

  app.route<{ Params: { name: string } }>({
    method: 'POST',
    url: '/api/v1/gpu-devices/:name/unassign',
    preHandler: requireAuth,
    schema: {
      summary: 'Unassign a GPU device',
      tags: ['gpu-devices'],
      security,
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'GpuDevice', 'unassign')) return forbid(reply);
      try {
        await resource.patch(req.params.name, { spec: { assignedTo: null } });
        return accepted({ message: `unassigned ${req.params.name}` });
      } catch (err) {
        if (isNotFound(err)) return notFound(reply, req.params.name);
        throw err;
      }
    },
  });
}

export async function gpuDevicesRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerImpl(app, db);
    registerGpuActions(app, buildGpuDeviceResource(db));
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/gpu-devices', summary: 'List GPU devices', tag: 'gpu-devices' },
    {
      method: 'GET',
      url: '/api/v1/gpu-devices/:name',
      summary: 'Get a GPU device',
      tag: 'gpu-devices',
    },
  ]);
}
