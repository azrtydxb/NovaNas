import type { CustomObjectsApi } from '@kubernetes/client-node';
import { type App, AppSchema } from '@novanas/schemas';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canRead } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import {
  CrdApiError,
  CrdConflictError,
  CrdInvalidError,
  CrdNotFoundError,
  CrdResource,
} from '../services/crd.js';
import type { AuthenticatedUser } from '../types.js';

/**
 * `App` CRs are synthesized from `AppCatalog` sources — they represent
 * observed catalog state, not user-authored configuration. We expose only
 * read endpoints; writes return 405 Method Not Allowed.
 */
export function buildAppResource(api: CustomObjectsApi): CrdResource<App> {
  return new CrdResource<App>({
    api,
    gvr: { group: 'novanas.io', version: 'v1alpha1', plural: 'apps' },
    schema: AppSchema,
    namespaced: false,
  });
}

function errorStatus(err: unknown): number {
  if (err instanceof CrdNotFoundError) return 404;
  if (err instanceof CrdConflictError) return 409;
  if (err instanceof CrdInvalidError) return 422;
  if (err instanceof CrdApiError) return err.statusCode || 500;
  return 500;
}

function errorBody(err: unknown): { error: string; message: string } {
  if (err instanceof CrdApiError) {
    return { error: err.name, message: err.message };
  }
  const msg = (err as { message?: string })?.message ?? 'internal error';
  return { error: 'internal_error', message: msg };
}

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function methodNotAllowed(reply: FastifyReply): FastifyReply {
  return reply.code(405).send({
    error: 'method_not_allowed',
    message: 'App entries are synthesized from AppCatalog sources and are read-only',
  });
}

export function register(app: FastifyInstance, api: CustomObjectsApi): void {
  const resource = buildAppResource(api);
  const security = [{ sessionCookie: [] }];
  const basePath = '/api/v1/apps-available';
  const tag = 'apps-available';

  app.route({
    method: 'GET',
    url: basePath,
    preHandler: requireAuth,
    schema: { summary: 'List available apps from catalogs', tags: [tag], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canRead(user, 'App')) return forbid(reply);
      try {
        const out = await resource.list();
        return { items: out.items };
      } catch (err) {
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });

  app.route<{ Params: { name: string } }>({
    method: 'GET',
    url: `${basePath}/:name`,
    preHandler: requireAuth,
    schema: {
      summary: 'Get an available app',
      tags: [tag],
      security,
      params: {
        type: 'object',
        properties: { name: { type: 'string' } },
        required: ['name'],
      },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canRead(user, 'App')) return forbid(reply);
      try {
        return await resource.get(req.params.name);
      } catch (err) {
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });

  // Explicit 405 handlers for mutating methods.
  for (const method of ['POST', 'PATCH', 'DELETE'] as const) {
    app.route({
      method,
      url: method === 'POST' ? basePath : `${basePath}/:name`,
      preHandler: requireAuth,
      schema: {
        summary: 'Not allowed — apps-available is read-only',
        tags: [tag],
        security,
      },
      handler: async (_req, reply) => methodNotAllowed(reply),
    });
  }
}
