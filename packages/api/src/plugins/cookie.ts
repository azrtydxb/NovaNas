import cookie from '@fastify/cookie';
import type { FastifyInstance } from 'fastify';
import type { Env } from '../env.js';

export async function registerCookie(app: FastifyInstance, env: Env): Promise<void> {
  await app.register(cookie, {
    secret: env.SESSION_SECRET,
    hook: 'onRequest',
    parseOptions: {
      httpOnly: true,
      sameSite: 'lax',
      secure: env.NODE_ENV === 'production',
      path: '/',
    },
  });
}
