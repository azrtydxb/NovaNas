import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { register as registerApps } from '../resources/apps.js';
import {
  accepted,
  kubeErrorReply,
  nowIso,
  patchSpec,
  requireDestructiveConfirm,
  setAnnotation,
} from '../services/actions.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

const GVR = { group: 'novanas.io', version: 'v1alpha1', plural: 'appinstances' };

function forbid(reply: Parameters<typeof kubeErrorReply>[0]) {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function registerAppActions(app: FastifyInstance, api: CustomObjectsApi): void {
  const security = [{ sessionCookie: [] }];

  // POST /api/v1/apps/:namespace/:name/start
  app.route<{ Params: { namespace: string; name: string } }>({
    method: 'POST',
    url: '/api/v1/apps/:namespace/:name/start',
    preHandler: requireAuth,
    schema: { summary: 'Start an app instance', tags: ['apps'], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const { namespace, name } = req.params;
      if (!canAction(user, 'AppInstance', 'start', namespace)) return forbid(reply);
      try {
        await patchSpec(api, GVR, name, { spec: { desiredState: 'Running' } }, namespace);
        return accepted({ message: `start requested for ${name}` });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });

  // POST /api/v1/apps/:namespace/:name/stop
  app.route<{ Params: { namespace: string; name: string } }>({
    method: 'POST',
    url: '/api/v1/apps/:namespace/:name/stop',
    preHandler: requireAuth,
    schema: { summary: 'Stop an app instance', tags: ['apps'], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const { namespace, name } = req.params;
      if (!canAction(user, 'AppInstance', 'stop', namespace)) return forbid(reply);
      try {
        await patchSpec(api, GVR, name, { spec: { desiredState: 'Stopped' } }, namespace);
        return accepted({ message: `stop requested for ${name}` });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });

  // POST /api/v1/apps/:namespace/:name/update — body: { version: string }
  app.route<{
    Params: { namespace: string; name: string };
    Body: { version?: string };
  }>({
    method: 'POST',
    url: '/api/v1/apps/:namespace/:name/update',
    preHandler: requireAuth,
    schema: {
      summary: 'Update an app to a new version',
      tags: ['apps'],
      security,
      body: {
        type: 'object',
        properties: { version: { type: 'string' } },
        required: ['version'],
      },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const { namespace, name } = req.params;
      if (!canAction(user, 'AppInstance', 'update', namespace)) return forbid(reply);
      const version = req.body?.version;
      if (!version || typeof version !== 'string') {
        return reply
          .code(400)
          .send({ error: 'invalid_body', message: 'version (string) required' });
      }
      try {
        await patchSpec(api, GVR, name, { spec: { version } }, namespace);
        await setAnnotation(api, GVR, name, 'novanas.io/action-update', nowIso(), namespace);
        return accepted({ message: `update to ${version} requested` });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });

  // DELETE /api/v1/apps/:namespace/:name?deleteData=true|false
  app.route<{
    Params: { namespace: string; name: string };
    Querystring: { deleteData?: string; confirm?: string };
  }>({
    method: 'DELETE',
    url: '/api/v1/apps/:namespace/:name',
    preHandler: requireAuth,
    schema: {
      summary: 'Uninstall an app',
      tags: ['apps'],
      security,
      querystring: {
        type: 'object',
        properties: {
          deleteData: { type: 'string' },
          confirm: { type: 'string' },
        },
      },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const { namespace, name } = req.params;
      if (!canAction(user, 'AppInstance', 'delete', namespace)) return forbid(reply);
      const deleteData = req.query.deleteData === 'true';
      if (deleteData && !requireDestructiveConfirm(req, reply, name)) return reply;
      try {
        const { group, version, plural } = GVR;
        await api.deleteNamespacedCustomObject(group, version, namespace, plural, name);
        const warnings: string[] = [];
        if (deleteData) warnings.push('persistent volumes for this app will be deleted');
        return accepted({
          message: `delete requested for ${name}`,
          status: 'running',
          warnings: warnings.length ? warnings : undefined,
        });
      } catch (err) {
        return kubeErrorReply(reply, err);
      }
    },
  });
}

export async function appRoutes(app: FastifyInstance, api?: CustomObjectsApi): Promise<void> {
  if (api) {
    registerApps(app, api);
    registerAppActions(app, api);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/apps', summary: 'List installed apps', tag: 'apps' },
    { method: 'POST', url: '/api/v1/apps', summary: 'Install an app', tag: 'apps' },
    { method: 'GET', url: '/api/v1/apps/:name', summary: 'Get an app', tag: 'apps' },
    { method: 'PATCH', url: '/api/v1/apps/:name', summary: 'Update app config', tag: 'apps' },
    { method: 'DELETE', url: '/api/v1/apps/:name', summary: 'Uninstall an app', tag: 'apps' },
  ]);
}
