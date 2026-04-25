import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify';
import { hasRole } from './rbac.js';

/**
 * Fastify decorators for `request.user`. The `auth` plugin populates
 * `request.user` on the preHandler lifecycle if a valid session cookie
 * is present. These helpers enforce presence on individual routes.
 */

export function requireAuth(
  req: FastifyRequest,
  reply: FastifyReply,
  done: (err?: Error) => void
): void {
  if (!req.user) {
    reply.code(401).send({ error: 'unauthorized', message: 'authentication required' });
    return;
  }
  done();
}

export function requireRole(role: string) {
  return (req: FastifyRequest, reply: FastifyReply, done: (err?: Error) => void): void => {
    if (!req.user) {
      reply.code(401).send({ error: 'unauthorized' });
      return;
    }
    if (!hasRole(req.user, role)) {
      reply.code(403).send({ error: 'forbidden', message: `missing role: ${role}` });
      return;
    }
    done();
  };
}

export function registerAuthDecorators(app: FastifyInstance): void {
  app.decorateRequest('user', undefined);
  app.decorateRequest('sessionId', undefined);
}
