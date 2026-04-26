import { randomBytes } from 'node:crypto';
import type { DbClient } from '../services/db.js';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { register as registerUsers } from '../resources/users.js';
import { accepted } from '../services/actions.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function registerUserActions(app: FastifyInstance): void {
  const security = [{ sessionCookie: [] }];

  // POST /api/v1/users/:name/reset-password (admin only; sends email via Keycloak)
  app.route<{ Params: { name: string } }>({
    method: 'POST',
    url: '/api/v1/users/:name/reset-password',
    preHandler: requireAuth,
    schema: {
      summary: 'Trigger a password reset email via Keycloak',
      tags: ['users'],
      security,
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      // User kind is admin-only-write, so canAction yields true only for admins.
      if (!canAction(user, 'User', 'reset-password')) return forbid(reply);
      // Operator-side TODO: actually dispatch a Keycloak admin API call
      // (executeActionsEmail with UPDATE_PASSWORD). For now we record the
      // intent — the operator/controller reconciler will perform the call.
      return accepted({
        status: 'pending',
        message: `password reset email requested for ${req.params.name}`,
      });
    },
  });

  // POST /api/v1/users/:name/enroll-2fa
  app.route<{ Params: { name: string } }>({
    method: 'POST',
    url: '/api/v1/users/:name/enroll-2fa',
    preHandler: requireAuth,
    schema: {
      summary: 'Enroll a user in TOTP 2FA. Returns secret + otpauth URL.',
      tags: ['users'],
      security,
      response: {
        200: {
          type: 'object',
          properties: {
            accepted: { type: 'boolean' },
            status: { type: 'string' },
            secret: { type: 'string' },
            otpauthUrl: { type: 'string' },
            message: { type: 'string' },
          },
          required: ['accepted', 'status', 'secret', 'otpauthUrl'],
        },
      },
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const targetUser = req.params.name;
      // A user can enroll themselves in 2FA; admins can enroll anyone.
      if (!canAction(user, 'User', 'enroll-2fa') && user.username !== targetUser) {
        return forbid(reply);
      }
      // Base32-encoded 20-byte secret (RFC 6238).
      const secret = randomBytes(20)
        .toString('base64')
        .replace(/=+$/, '')
        .replace(/[^A-Z2-7]/gi, 'A')
        .slice(0, 32)
        .toUpperCase();
      const issuer = 'NovaNas';
      const otpauthUrl = `otpauth://totp/${issuer}:${encodeURIComponent(
        targetUser
      )}?secret=${secret}&issuer=${issuer}&algorithm=SHA1&digits=6&period=30`;
      // Operator-side TODO: register this credential in Keycloak. Until then
      // the secret is issued but not yet activated server-side.
      return {
        ...accepted({ status: 'pending', message: '2FA enrollment initiated' }),
        secret,
        otpauthUrl,
      };
    },
  });
}

export async function userRoutes(app: FastifyInstance, db?: DbClient | null): Promise<void> {
  if (db) {
    registerUsers(app, db);
    registerUserActions(app);
    return;
  }
  registerUnavailable(app, [
    { method: 'GET', url: '/api/v1/users', summary: 'List users', tag: 'users' },
    { method: 'POST', url: '/api/v1/users', summary: 'Create a user', tag: 'users' },
    { method: 'GET', url: '/api/v1/users/:name', summary: 'Get a user', tag: 'users' },
    { method: 'PATCH', url: '/api/v1/users/:name', summary: 'Update a user', tag: 'users' },
    { method: 'DELETE', url: '/api/v1/users/:name', summary: 'Delete a user', tag: 'users' },
  ]);
}
