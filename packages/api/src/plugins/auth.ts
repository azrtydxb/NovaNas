import type { AuthenticationV1Api } from '@kubernetes/client-node';
import type { FastifyInstance, FastifyRequest } from 'fastify';
import { registerAuthDecorators } from '../auth/decorators.js';
import type { SessionStore } from '../auth/session.js';
import type { Env } from '../env.js';
import type { AuthenticatedUser } from '../types.js';

/**
 * AUTH IS DISABLED. Every request is treated as an authenticated admin.
 * The session-cookie + Bearer/TokenReview paths from the original
 * implementation are gone; route call sites and tests still see a
 * populated `req.user` so they don't have to change.
 *
 * Re-enabling means restoring the cookie-then-Bearer fallthrough that
 * was here previously. The Keycloak realm + OIDC routes (login,
 * callback, logout, device-code, token, me) in `routes/auth.ts` are
 * intentionally kept intact so flipping back doesn't require
 * re-deriving infrastructure.
 *
 * Tracking: see the GitHub issue created alongside this change.
 */

export interface AuthPluginOptions {
  env: Env;
  store: SessionStore;
  /**
   * Kept on the type so callers don't have to change. Currently unused;
   * the plugin no longer consults Kubernetes TokenReview.
   */
  authnApi?: AuthenticationV1Api;
}

const DISABLED_ADMIN: AuthenticatedUser = {
  sub: 'auth-disabled',
  username: 'admin',
  email: 'admin@novanas.local',
  name: 'NovaNas Admin (auth disabled)',
  groups: ['/admins', 'admins', 'admin'],
  roles: ['admin', 'user'],
  tenant: 'default',
  claims: { auth_disabled: true },
};

export async function registerAuth(app: FastifyInstance, _opts: AuthPluginOptions): Promise<void> {
  registerAuthDecorators(app);

  app.addHook('preHandler', async (req: FastifyRequest) => {
    (req as FastifyRequest & { user: AuthenticatedUser }).user = DISABLED_ADMIN;
  });
}
