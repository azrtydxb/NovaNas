import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify';
import type { z } from 'zod';
import { type Kind, canDelete, canRead, canWrite, ownNamespace } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { writeAudit } from '../services/audit.js';
import { CrdApiError } from '../services/crd.js';
import {
  isConflict as isResourceConflict,
  isInvalid as isResourceInvalid,
  isNotFound as isResourceNotFound,
  isResourceApiError,
  type Resource,
} from '../services/resource.js';
import type { AuthenticatedUser } from '../types.js';

/**
 * Wires the five canonical CRUD routes (list/get/create/patch/delete) for
 * a CRD kind onto a Fastify instance with auth + authz + audit integration.
 */

export interface RegisterOptions<T> {
  app: FastifyInstance;
  /** Base URL path, e.g. `/api/v1/pools`. */
  basePath: string;
  /** OpenAPI tag. */
  tag: string;
  /** Resource kind (used for authz + audit). */
  kind: Kind;
  /** Backing resource (CRD or Postgres-backed; routes don't care). */
  resource: Resource<T>;
  /** Zod schema for responses + payload validation. */
  schema: z.ZodType<T>;
  /**
   * Namespace resolver for namespaced resources. If omitted, the resource
   * is treated as cluster-scoped.
   */
  resolveNamespace?: (req: FastifyRequest, user: AuthenticatedUser) => string;
  /**
   * Optional async hook fired BEFORE a CREATE/PATCH/DELETE call hits
   * Kubernetes. Use it to enforce domain rules that the Zod schema
   * can't express on its own (cross-resource invariants like "no two
   * pools share a tier"). Throw `RegisterValidationError` to short-
   * circuit with a 422 response carrying your message.
   */
  validate?: (
    action: 'create' | 'patch' | 'delete',
    body: unknown,
    req: FastifyRequest
  ) => Promise<void> | void;
}

/** Thrown by RegisterOptions.validate hooks to surface a 422 with a message. */
export class RegisterValidationError extends Error {
  constructor(message: string) {
    super(message);
    this.name = 'RegisterValidationError';
  }
}

function errorStatus(err: unknown): number {
  if (isResourceNotFound(err)) return 404;
  if (isResourceConflict(err)) return 409;
  if (isResourceInvalid(err)) return 422;
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

export function registerCrudRoutes<T>(opts: RegisterOptions<T>): void {
  const { app, basePath, tag, kind, resource, resolveNamespace } = opts;
  const security = [{ sessionCookie: [] }];

  // LIST
  app.route({
    method: 'GET',
    url: basePath,
    preHandler: requireAuth,
    schema: {
      summary: `List ${kind}s`,
      tags: [tag],
      security,
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const namespace = resolveNamespace?.(req, user);
      if (!canRead(user, kind, namespace)) return forbid(reply);
      try {
        const out = await resource.list({ namespace });
        return { items: out.items };
      } catch (err) {
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });

  // GET :name
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
      const namespace = resolveNamespace?.(req, user);
      if (!canRead(user, kind, namespace)) return forbid(reply);
      try {
        return await resource.get(req.params.name, namespace);
      } catch (err) {
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });

  // POST create
  app.route({
    method: 'POST',
    url: basePath,
    preHandler: requireAuth,
    schema: {
      summary: `Create a ${kind}`,
      tags: [tag],
      security,
      body: { type: 'object' },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const namespace = resolveNamespace?.(req, user);
      if (!canWrite(user, kind, namespace)) return forbid(reply);
      // The route already knows the resource kind/apiVersion; the
      // client only has to send `{metadata, spec}` and we hydrate the
      // K8s envelope before validation. Keeps the JSON the SPA posts
      // human-shaped instead of forcing it to know CRD details.
      const incoming = (req.body ?? {}) as Record<string, unknown>;
      const hydrated = {
        apiVersion: 'novanas.io/v1alpha1',
        kind,
        ...incoming,
      };
      const parsed = opts.schema.safeParse(hydrated);
      if (!parsed.success) {
        return reply.code(400).send({ error: 'invalid_body', message: parsed.error.message });
      }
      if (opts.validate) {
        try {
          await opts.validate('create', parsed.data, req);
        } catch (err) {
          if (err instanceof RegisterValidationError) {
            return reply.code(422).send({ error: 'invalid_request', message: err.message });
          }
          throw err;
        }
      }
      const name =
        (parsed.data as unknown as { metadata?: { name?: string } })?.metadata?.name ?? '';
      try {
        const created = await resource.create(parsed.data, namespace);
        await writeAudit(null, req.log, {
          actor: user.username,
          action: 'create',
          kind,
          resource: kind,
          resourceId: name,
          namespace,
          outcome: 'success',
          ip: req.ip,
        });
        return reply.code(201).send(created);
      } catch (err) {
        await writeAudit(null, req.log, {
          actor: user.username,
          action: 'create',
          kind,
          resource: kind,
          resourceId: name,
          namespace,
          outcome: 'failure',
          ip: req.ip,
        });
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });

  // PATCH :name
  app.route<{ Params: { name: string }; Body: Record<string, unknown> }>({
    method: 'PATCH',
    url: `${basePath}/:name`,
    preHandler: requireAuth,
    schema: {
      summary: `Update a ${kind}`,
      tags: [tag],
      security,
      params: {
        type: 'object',
        properties: { name: { type: 'string' } },
        required: ['name'],
      },
      body: { type: 'object' },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const namespace = resolveNamespace?.(req, user);
      if (!canWrite(user, kind, namespace)) return forbid(reply);
      if (!req.body || typeof req.body !== 'object') {
        return reply.code(400).send({ error: 'invalid_body', message: 'object required' });
      }
      if (opts.validate) {
        try {
          await opts.validate('patch', req.body, req);
        } catch (err) {
          if (err instanceof RegisterValidationError) {
            return reply.code(422).send({ error: 'invalid_request', message: err.message });
          }
          throw err;
        }
      }
      try {
        const updated = await resource.patch(req.params.name, req.body, namespace);
        await writeAudit(null, req.log, {
          actor: user.username,
          action: 'update',
          kind,
          resource: kind,
          resourceId: req.params.name,
          namespace,
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
          resourceId: req.params.name,
          namespace,
          outcome: 'failure',
          ip: req.ip,
        });
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });

  // DELETE :name
  app.route<{ Params: { name: string } }>({
    method: 'DELETE',
    url: `${basePath}/:name`,
    preHandler: requireAuth,
    schema: {
      summary: `Delete a ${kind}`,
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
      const namespace = resolveNamespace?.(req, user);
      if (!canDelete(user, kind, namespace)) return forbid(reply);
      try {
        await resource.delete(req.params.name, namespace);
        await writeAudit(null, req.log, {
          actor: user.username,
          action: 'delete',
          kind,
          resource: kind,
          resourceId: req.params.name,
          namespace,
          outcome: 'success',
          ip: req.ip,
        });
        return reply.code(204).send();
      } catch (err) {
        await writeAudit(null, req.log, {
          actor: user.username,
          action: 'delete',
          kind,
          resource: kind,
          resourceId: req.params.name,
          namespace,
          outcome: 'failure',
          ip: req.ip,
        });
        return reply.code(errorStatus(err)).send(errorBody(err));
      }
    },
  });
}

/** Resolver that forces namespaced resources into the user's own namespace. */
export function userNamespaceResolver(_req: FastifyRequest, user: AuthenticatedUser): string {
  return ownNamespace(user);
}
