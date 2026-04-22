import type { FastifyInstance, FastifySchema } from 'fastify';
import { requireAuth } from '../auth/decorators.js';

export const NOT_IMPLEMENTED = { error: 'not implemented', wave: 2 } as const;

export interface StubRoute {
  method: 'GET' | 'POST' | 'PUT' | 'PATCH' | 'DELETE';
  url: string;
  summary: string;
  tag: string;
}

export function registerStubs(app: FastifyInstance, routes: StubRoute[]): void {
  for (const r of routes) {
    const schema: FastifySchema = {
      summary: r.summary,
      description: r.summary,
      tags: [r.tag],
      security: [{ sessionCookie: [] }],
      response: {
        501: {
          type: 'object',
          properties: {
            error: { type: 'string' },
            wave: { type: 'number' },
          },
          required: ['error', 'wave'],
        },
      },
    };
    app.route({
      method: r.method,
      url: r.url,
      schema,
      preHandler: requireAuth,
      handler: async (_req, reply) => reply.code(501).send(NOT_IMPLEMENTED),
    });
  }
}
