import type { AuthenticationV1Api } from '@kubernetes/client-node';
import type { FastifyInstance, FastifyRequest } from 'fastify';
import { registerAuthDecorators } from '../auth/decorators.js';
import { userFromClaims } from '../auth/rbac.js';
import type { SessionStore } from '../auth/session.js';
import { buildTokenReviewMiddleware } from '../auth/tokenreview.js';
import type { Env } from '../env.js';
import type { AuthenticatedUser } from '../types.js';

/**
 * Authentication plugin. Attaches `request.user` from one of:
 *   1. A signed session cookie (browser SPA)
 *   2. A Bearer token validated via Kubernetes TokenReview (in-cluster
 *      service accounts: disk-agent, storage-meta, …)
 *
 * Does NOT enforce — routes opt in via `requireAuth` preHandler.
 */
export interface AuthPluginOptions {
  env: Env;
  store: SessionStore;
  /**
   * Kubernetes Authentication v1 client. When omitted, Bearer tokens
   * are not validated and only session-cookie auth is available
   * (useful for tests).
   */
  authnApi?: AuthenticationV1Api;
}

export async function registerAuth(app: FastifyInstance, opts: AuthPluginOptions): Promise<void> {
  const { env, store, authnApi } = opts;
  registerAuthDecorators(app);

  const tokenReview = authnApi ? buildTokenReviewMiddleware({ api: authnApi }) : null;

  app.addHook('preHandler', async (req: FastifyRequest, reply) => {
    // 1. Session cookie (browser).
    const sid = req.cookies?.[env.SESSION_COOKIE_NAME];
    if (sid) {
      const unsigned = req.unsignCookie(sid);
      if (unsigned.valid && unsigned.value) {
        const session = await store.touch(unsigned.value);
        if (session) {
          req.sessionId = unsigned.value;
          req.user = userFromClaims(session.claims);
          return;
        }
      }
    }
    // 2. Bearer token (in-cluster service account). Only attempt if
    //    the header is present so unauthenticated SPA requests don't
    //    pay the TokenReview round-trip.
    const authz = req.headers.authorization ?? '';
    if (tokenReview && authz.toLowerCase().startsWith('bearer ')) {
      // tokenReview.authenticate writes `req.user` on success or
      // sends a 401/403 reply itself on failure. Either way, downstream
      // handlers won't run if reply was sent.
      await tokenReview.authenticate(req, reply);
      const u = (req as FastifyRequest & { user?: AuthenticatedUser }).user;
      if (u) return;
    }
  });
}
