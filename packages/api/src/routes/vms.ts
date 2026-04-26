import type { Vm } from '@novanas/schemas';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { buildVmResource, register as registerVms } from '../resources/vms.js';
import {
  accepted,
  nowIso,
  requireDestructiveConfirm,
  setAnnotationOnResource,
} from '../services/actions.js';
import type { DbClient } from '../services/db.js';
import type { PgResource } from '../services/pg-resource.js';
import { isNotFound } from '../services/resource.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function notFound(reply: FastifyReply, name: string): FastifyReply {
  return reply.code(404).send({ error: 'not_found', message: `vm '${name}' not found` });
}

function registerVmActions(app: FastifyInstance, resource: PgResource<Vm>): void {
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
          await resource.patch(name, buildPatch(force), namespace);
          if (annotation) {
            await setAnnotationOnResource(resource, name, annotation, nowIso(), namespace);
          }
          return accepted({ message: `${action} requested for ${name}` });
        } catch (err) {
          if (isNotFound(err)) return notFound(reply, name);
          throw err;
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
        await resource.delete(name, namespace);
        const warnings = deleteDisks ? ['attached disks will be deleted'] : undefined;
        return accepted({ message: `delete requested for ${name}`, warnings });
      } catch (err) {
        if (isNotFound(err)) return notFound(reply, name);
        throw err;
      }
    },
  });
}

export async function vmRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerVms(app, db);
    registerVmActions(app, buildVmResource(db));
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/vms', summary: 'List VMs', tag: 'vms' },
    { method: 'POST', url: '/api/v1/vms', summary: 'Create a VM', tag: 'vms' },
    { method: 'GET', url: '/api/v1/vms/:name', summary: 'Get a VM', tag: 'vms' },
    { method: 'PATCH', url: '/api/v1/vms/:name', summary: 'Update a VM', tag: 'vms' },
    { method: 'DELETE', url: '/api/v1/vms/:name', summary: 'Delete a VM', tag: 'vms' },
  ]);
}
