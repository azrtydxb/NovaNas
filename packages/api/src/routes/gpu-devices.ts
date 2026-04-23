import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { register as registerImpl } from '../resources/gpu-devices.js';
import { accepted, kubeErrorReply, patchSpec } from '../services/actions.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

const GVR = { group: 'novanas.io', version: 'v1alpha1', plural: 'gpudevices' };

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function registerGpuActions(app: FastifyInstance, api: CustomObjectsApi): void {
  const security = [{ sessionCookie: [] }];

  // POST /api/v1/gpu-devices/:name/assign — body: { vmNamespace, vmName }
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
        await patchSpec(api, GVR, req.params.name, {
          spec: { assignedTo: { namespace: vmNamespace, name: vmName } },
        });
        return accepted({ message: `assigned to ${vmNamespace}/${vmName}` });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });

  // POST /api/v1/gpu-devices/:name/unassign
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
        await patchSpec(api, GVR, req.params.name, { spec: { assignedTo: null } });
        return accepted({ message: `unassigned ${req.params.name}` });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });
}

export async function gpuDevicesRoutes(
  app: FastifyInstance,
  api?: CustomObjectsApi
): Promise<void> {
  if (api) {
    registerImpl(app, api);
    registerGpuActions(app, api);
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
