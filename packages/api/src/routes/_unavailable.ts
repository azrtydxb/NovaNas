import type { FastifyInstance, FastifySchema } from 'fastify';
import { requireAuth } from '../auth/decorators.js';

/**
 * Response body when a CRD-backed route is registered in an environment
 * that has no Kubernetes CustomObjects client (e.g. unit tests without a
 * kubeconfig). Production deployments always pass `kubeCustom`, so these
 * fallback handlers are never hit in real clusters.
 */
export const KUBE_UNAVAILABLE = {
  error: 'kube_unavailable',
  message: 'Kubernetes CustomObjects client is not configured',
} as const;

export interface UnavailableRoute {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  url: string;
  summary: string;
  tag: string;
}

/**
 * Register placeholder handlers that reply with HTTP 503 + a structured JSON
 * body. Used by CRD-backed route modules when no `kubeCustom` is supplied.
 * The real CRUD handlers (backed by @novanas/schemas + kubeCustom) are
 * registered instead whenever a client is available.
 */
export function registerUnavailable(app: FastifyInstance, routes: UnavailableRoute[]): void {
  for (const r of routes) {
    const schema: FastifySchema = {
      summary: r.summary,
      description: r.summary,
      tags: [r.tag],
      security: [{ sessionCookie: [] }],
      response: {
        503: {
          type: 'object',
          properties: {
            error: { type: 'string' },
            message: { type: 'string' },
          },
          required: ['error', 'message'],
        },
      },
    };
    app.route({
      method: r.method,
      url: r.url,
      schema,
      preHandler: requireAuth,
      handler: async (_req, reply) => reply.code(503).send(KUBE_UNAVAILABLE),
    });
  }
}
