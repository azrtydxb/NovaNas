import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { register as registerVms } from '../resources/vms.js';
import {
  accepted,
  kubeErrorReply,
  nowIso,
  patchSpec,
  requireDestructiveConfirm,
  setAnnotation,
} from '../services/actions.js';
import type { AuthenticatedUser } from '../types.js';
import { registerStubs } from './_stubs.js';

const GVR = { group: 'novanas.io', version: 'v1alpha1', plural: 'vms' };

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function registerVmActions(app: FastifyInstance, api: CustomObjectsApi): void {
  const security = [{ sessionCookie: [] }];

  const powerAction = (
    url: string,
    summary: string,
    action: string,
    buildPatch: (force: boolean) => Record<string, unknown>,
    annotation?: string
  ) => {
    app.route<{
      Params: { namespace: string; name: string };
      Querystring: { force?: string };
    }>({
      method: 'POST',
      url,
      preHandler: requireAuth,
      schema: { summary, tags: ['vms'], security },
      handler: async (req, reply) => {
        const user = req.user as AuthenticatedUser;
        const { namespace, name } = req.params;
        if (!canAction(user, 'Vm', action, namespace)) return forbid(reply);
        const force = req.query.force === 'true';
        try {
          const patch = buildPatch(force);
          await patchSpec(api, GVR, name, patch, namespace);
          if (annotation) {
            await setAnnotation(api, GVR, name, annotation, nowIso(), namespace);
          }
          return accepted({ message: `${action} requested for ${name}` });
        } catch (err) {
          return kubeErrorReply(reply, err);
        }
      },
    });
  };

  powerAction('/api/v1/vms/:namespace/:name/start', 'Start a VM', 'start', () => ({
    spec: { powerState: 'Running' },
  }));
  powerAction(
    '/api/v1/vms/:namespace/:name/stop',
    'Stop a VM (ACPI, or force=true for power-off)',
    'stop',
    (force) => ({
      spec: { powerState: force ? 'Off' : 'Stopped' },
    })
  );
  powerAction(
    '/api/v1/vms/:namespace/:name/reset',
    'Hard reset a VM',
    'reset',
    () => ({ spec: { powerState: 'Running' } }),
    'novanas.io/action-reset'
  );
  powerAction('/api/v1/vms/:namespace/:name/pause', 'Pause a VM', 'pause', () => ({
    spec: { powerState: 'Paused' },
  }));
  powerAction('/api/v1/vms/:namespace/:name/resume', 'Resume a paused VM', 'resume', () => ({
    spec: { powerState: 'Running' },
  }));

  // DELETE /api/v1/vms/:namespace/:name?deleteDisks=true|false
  app.route<{
    Params: { namespace: string; name: string };
    Querystring: { deleteDisks?: string; confirm?: string };
  }>({
    method: 'DELETE',
    url: '/api/v1/vms/:namespace/:name',
    preHandler: requireAuth,
    schema: {
      summary: 'Delete a VM',
      tags: ['vms'],
      security,
      querystring: {
        type: 'object',
        properties: {
          deleteDisks: { type: 'string' },
          confirm: { type: 'string' },
        },
      },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const { namespace, name } = req.params;
      if (!canAction(user, 'Vm', 'delete', namespace)) return forbid(reply);
      const deleteDisks = req.query.deleteDisks === 'true';
      if (deleteDisks && !requireDestructiveConfirm(req, reply, name)) return reply;
      try {
        const { group, version, plural } = GVR;
        await api.deleteNamespacedCustomObject(group, version, namespace, plural, name);
        const warnings = deleteDisks ? ['attached disks will be deleted'] : undefined;
        return accepted({ message: `delete requested for ${name}`, warnings });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });
}

export async function vmRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerVms(app, api);
    registerVmActions(app, api);
    return;
  }
  registerStubs(app, [
    { method: 'GET', url: '/api/v1/vms', summary: 'List VMs', tag: 'vms' },
    { method: 'POST', url: '/api/v1/vms', summary: 'Create a VM', tag: 'vms' },
    { method: 'GET', url: '/api/v1/vms/:name', summary: 'Get a VM', tag: 'vms' },
    { method: 'PATCH', url: '/api/v1/vms/:name', summary: 'Update a VM', tag: 'vms' },
    { method: 'DELETE', url: '/api/v1/vms/:name', summary: 'Delete a VM', tag: 'vms' },
  ]);
}
