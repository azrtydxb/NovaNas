import { type App, AppSchema } from '@novanas/schemas';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canRead } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import type { DbClient } from '../services/db.js';
import { PgResource } from '../services/pg-resource.js';
import { isNotFound } from '../services/resource.js';
import type { AuthenticatedUser } from '../types.js';

/**
 * `App` is a synthesized catalog reflection — it represents observed
 * catalog state, not user-authored configuration. Reads only; writes
 * return 405.
 */
export function buildAppResource(db: DbClient): PgResource<App> {
  return new PgResource<App>({
    db,
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'App',
    schema: AppSchema,
    namespaced: false,
  });
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

export function register(app: FastifyInstance, db: DbClient): void {
  const resource = buildAppResource(db);
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
      const out = await resource.list();
      return { items: out.items };
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
        if (isNotFound(err)) {
          return reply
            .code(404)
            .send({ error: 'not_found', message: `app '${req.params.name}' not found` });
        }
        throw err;
      }
    },
  });

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
