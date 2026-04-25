import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify';
import type { z } from 'zod';
import { type Kind, canDelete, canRead, canWrite, ownNamespace } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { writeAudit } from '../services/audit.js';
import {
  CrdApiError,
  CrdConflictError,
  CrdInvalidError,
  CrdNotFoundError,
  type CrdResource,
} from '../services/crd.js';
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
  /** Backing CRD resource. */
  resource: CrdResource<T>;
  /** Zod schema for responses + payload validation. */
  schema: z.ZodType<T>;
  /**
   * Namespace resolver for namespaced resources. If omitted, the resource
   * is treated as cluster-scoped.
   */
  resolveNamespace?: (req: FastifyRequest, user: AuthenticatedUser) => string;
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
