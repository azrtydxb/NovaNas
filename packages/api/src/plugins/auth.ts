import type { FastifyInstance, FastifyRequest } from 'fastify';
import { registerAuthDecorators } from '../auth/decorators.js';
import { userFromClaims } from '../auth/rbac.js';
import type { SessionStore } from '../auth/session.js';
import type { Env } from '../env.js';

/**
 * Authentication plugin. Attaches `request.user` if a valid session
 * cookie is present. Does NOT enforce — routes opt in via
 * `requireAuth` preHandler.
 */
export async function registerAuth(
  app: FastifyInstance,
  env: Env,
  store: SessionStore
): Promise<void> {
  registerAuthDecorators(app);

  app.addHook('preHandler', async (req: FastifyRequest) => {
    const sid = req.cookies?.[env.SESSION_COOKIE_NAME];
    if (!sid) return;
    const unsigned = req.unsignCookie(sid);
    if (!unsigned.valid || !unsigned.value) return;
    const session = await store.touch(unsigned.value);
    if (!session) return;
    req.sessionId = unsigned.value;
    req.user = userFromClaims(session.claims);
  });
}
