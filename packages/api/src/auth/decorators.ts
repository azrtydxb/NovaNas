import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify';

/**
 * AUTH IS DISABLED. The pre-handlers below are no-ops; `plugins/auth.ts`
 * always populates `request.user` with a synthetic admin so call sites
 * can keep their `req.user` references unchanged. Re-enabling means
 * restoring the cookie/Bearer paths in `plugins/auth.ts` and turning
 * these pre-handlers back into real checks.
 */

export function requireAuth(
  _req: FastifyRequest,
  _reply: FastifyReply,
  done: (err?: Error) => void
): void {
  done();
}

export function requireRole(_role: string) {
  return (_req: FastifyRequest, _reply: FastifyReply, done: (err?: Error) => void): void => {
    done();
  };
}

export function registerAuthDecorators(app: FastifyInstance): void {
  app.decorateRequest('user', undefined);
  app.decorateRequest('sessionId', undefined);
}
