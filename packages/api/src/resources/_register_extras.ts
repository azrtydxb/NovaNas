import type { FastifyInstance, FastifyReply } from 'fastify';
import type { z } from 'zod';
import { type Kind, canRead, canWrite } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { writeAudit } from '../services/audit.js';
import { CrdApiError } from '../services/crd.js';
import { isResourceApiError, isNotFound as isResourceNotFound } from '../services/resource.js';
import type { AuthenticatedUser } from '../types.js';

/**
 * Extra registrars for CRD kinds that don't follow the vanilla CRUD pattern:
 *
 *  - Singletons: one cluster-scoped instance named `default`. Exposed as
 *    `GET /<path>` and `PATCH /<path>` only. No list/create/delete.
 *  - Read-only: list + get work; POST/PATCH/DELETE return HTTP 405.
 */

function errorStatus(err: unknown): number {
  if (isResourceNotFound(err)) return 404;
  if (err instanceof CrdApiError) return err.statusCode || 500;
  if (isResourceApiError(err)) return err.statusCode || 500;
  return 500;
}

function errorBody(err: unknown): { error: string; message: string } {
  if (isResourceApiError(err)) {
    return { error: err.name, message: err.message };
  }
  const msg = (err as { message?: string })?.message ?? 'internal error';
  return { error: 'internal_error', message: msg };
}

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

export interface SingletonOptions<T> {
  app: FastifyInstance;
  basePath: string;
  tag: string;
  kind: Kind;
  resource: import('../services/resource.js').Resource<T>;
  schema: z.ZodType<T>;
  /** The singleton's fixed metadata name. Defaults to `default`. */
  singletonName?: string;
}

/**
 * Registers `GET` + `PATCH` at `basePath` for a cluster-scoped singleton CRD.
 */
export function registerSingletonRoutes<T>(opts: SingletonOptions<T>): void {
  const { app, basePath, tag, kind, resource } = opts;
  const name = opts.singletonName ?? 'default';
  const security = [{ sessionCookie: [] }];

  app.route({
    method: 'GET',
    url: basePath,
    preHandler: requireAuth,
    schema: { summary: `Get ${kind}`, tags: [tag], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canRead(user, kind)) return forbid(reply);
      try {
        return await resource.get(name);
      } catch (err) {
        if (isResourceNotFound(err)) {
          return reply.code(404).send({
            error: 'not_found',
            message: `${kind} singleton '${name}' is not configured yet`,
          });
        }
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });

  app.route<{ Body: Record<string, unknown> }>({
    method: 'PATCH',
    url: basePath,
    preHandler: requireAuth,
    schema: { summary: `Update ${kind}`, tags: [tag], security, body: { type: 'object' } },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canWrite(user, kind)) return forbid(reply);
      if (!req.body || typeof req.body !== 'object') {
        return reply.code(400).send({ error: 'invalid_body', message: 'object required' });
      }
      try {
        const updated = await resource.patch(name, req.body);
        await writeAudit(null, req.log, {
          actor: user.username,
          action: 'update',
          kind,
          resource: kind,
          resourceId: name,
          outcome: 'success',
          ip: req.ip,
        });
        return updated;
      } catch (err) {
        await writeAudit(null, req.log, {
          actor: user.username,
          action: 'update',
          kind,
          resource: kind,
          resourceId: name,
          outcome: 'failure',
          ip: req.ip,
        });
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });
}

export interface ReadOnlyOptions<T> {
  app: FastifyInstance;
  basePath: string;
  tag: string;
  kind: Kind;
  resource: import('../services/resource.js').Resource<T>;
  schema: z.ZodType<T>;
}

/**
 * Registers list/get at `basePath`; POST/PATCH/DELETE return HTTP 405 with
 * a message noting the resource is observed rather than authored.
 */
export function registerReadOnlyRoutes<T>(opts: ReadOnlyOptions<T>): void {
  const { app, basePath, tag, kind, resource } = opts;
  const security = [{ sessionCookie: [] }];

  app.route({
    method: 'GET',
    url: basePath,
    preHandler: requireAuth,
    schema: { summary: `List ${kind}s`, tags: [tag], security },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canRead(user, kind)) return forbid(reply);
      try {
        const out = await resource.list({});
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
      summary: `Get a ${kind}`,
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
      if (!canRead(user, kind)) return forbid(reply);
      try {
        return await resource.get(req.params.name);
      } catch (err) {
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });

  const observed = {
    error: 'method_not_allowed',
    message: 'this resource is observed, not authored',
  };

  app.route({
    method: 'POST',
    url: basePath,
    preHandler: requireAuth,
    schema: { summary: `Create ${kind} (not allowed)`, tags: [tag], security },
    handler: async (_req, reply) => reply.code(405).send(observed),
  });
  app.route<{ Params: { name: string } }>({
    method: 'PATCH',
    url: `${basePath}/:name`,
    preHandler: requireAuth,
    schema: {
      summary: `Update ${kind} (not allowed)`,
      tags: [tag],
      security,
      params: {
        type: 'object',
        properties: { name: { type: 'string' } },
        required: ['name'],
      },
    },
    handler: async (_req, reply) => reply.code(405).send(observed),
  });
  app.route<{ Params: { name: string } }>({
    method: 'DELETE',
    url: `${basePath}/:name`,
    preHandler: requireAuth,
    schema: {
      summary: `Delete ${kind} (not allowed)`,
      tags: [tag],
      security,
      params: {
        type: 'object',
        properties: { name: { type: 'string' } },
        required: ['name'],
      },
    },
    handler: async (_req, reply) => reply.code(405).send(observed),
  });
}
