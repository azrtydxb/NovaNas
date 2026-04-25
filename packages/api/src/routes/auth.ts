import type { FastifyInstance, FastifyReply, FastifyRequest } from 'fastify';
import { z } from 'zod';
import { requireAuth } from '../auth/decorators.js';
import { userFromClaims } from '../auth/rbac.js';
import { SESSION_TTL_SECONDS, type SessionStore } from '../auth/session.js';
import type { Env } from '../env.js';
import type { KeycloakClient } from '../services/keycloak.js';
import type { SessionRecord } from '../types.js';

const OIDC_STATE_PREFIX = 'oidc:state:';
const OIDC_STATE_TTL = 10 * 60; // 10 minutes

const CallbackQuery = z.object({
  code: z.string().min(1),
  state: z.string().min(1),
});

const LoginBody = z.object({
  redirectTo: z.string().optional(),
});

const PasswordLoginBody = z.object({
  username: z.string().min(1).max(128),
  password: z.string().min(1).max(256),
});

export interface AuthRouteDeps {
  env: Env;
  keycloak: KeycloakClient;
  sessions: SessionStore;
  redis: import('ioredis').Redis;
}

export async function authRoutes(app: FastifyInstance, deps: AuthRouteDeps): Promise<void> {
  const { env, keycloak, sessions, redis } = deps;

  const callbackUrl = `${env.API_PUBLIC_URL.replace(/\/$/, '')}/api/v1/auth/callback`;

  // -- POST /api/v1/auth/login ------------------------------------------
  app.post(
    '/api/v1/auth/login',
    {
      schema: {
        description: 'Begin OIDC login. Returns the authorization URL.',
        tags: ['auth'],
        body: {
          type: 'object',
          properties: { redirectTo: { type: 'string' } },
          additionalProperties: false,
        },
        response: {
          200: {
            type: 'object',
            properties: { url: { type: 'string' } },
            required: ['url'],
          },
        },
      },
    },
    async (req: FastifyRequest, reply: FastifyReply) => {
      const body = LoginBody.parse(req.body ?? {});
      const auth = await keycloak.buildAuthUrl(callbackUrl);
      await redis.setex(
        `${OIDC_STATE_PREFIX}${auth.state}`,
        OIDC_STATE_TTL,
        JSON.stringify({
          nonce: auth.nonce,
          codeVerifier: auth.codeVerifier,
          redirectTo: body.redirectTo ?? '/',
        })
      );
      return reply.send({ url: auth.url.toString() });
    }
  );

  // -- GET /api/v1/auth/callback ----------------------------------------
  app.get(
    '/api/v1/auth/callback',
    {
      schema: {
        description: 'OIDC callback. Exchanges code for tokens, creates session.',
        tags: ['auth'],
        querystring: {
          type: 'object',
          required: ['code', 'state'],
          properties: {
            code: { type: 'string' },
            state: { type: 'string' },
            iss: { type: 'string' },
            session_state: { type: 'string' },
          },
        },
      },
    },
    async (req: FastifyRequest, reply: FastifyReply) => {
      const q = CallbackQuery.parse(req.query);
      const rawQ = req.query as Record<string, string | undefined>;
      const raw = await redis.get(`${OIDC_STATE_PREFIX}${q.state}`);
      if (!raw) {
        return reply.code(400).send({ error: 'invalid_state' });
      }
      await redis.del(`${OIDC_STATE_PREFIX}${q.state}`);
      const stored = JSON.parse(raw) as {
        nonce: string;
        codeVerifier: string;
        redirectTo: string;
      };

      const currentUrl = new URL(callbackUrl);
      currentUrl.searchParams.set('code', q.code);
      currentUrl.searchParams.set('state', q.state);
      // Forward optional response params produced by Keycloak
      // (`iss` is required by RFC 9207 and the openid-client library
      // refuses the exchange without it; `session_state` is harmless
      // but kept for parity with what KC actually sent).
      if (rawQ.iss) currentUrl.searchParams.set('iss', rawQ.iss);
      if (rawQ.session_state) currentUrl.searchParams.set('session_state', rawQ.session_state);

      const tokens = await keycloak.exchangeCode({
        currentUrl,
        state: q.state,
        nonce: stored.nonce,
        codeVerifier: stored.codeVerifier,
      });

      const claims = tokens.claims();
      if (!claims) {
        return reply.code(500).send({ error: 'no_id_token_claims' });
      }

      const user = userFromClaims(claims as Record<string, unknown>);
      const now = Date.now();
      const record: SessionRecord = {
        userId: user.sub,
        username: user.username,
        createdAt: now,
        expiresAt: now + SESSION_TTL_SECONDS * 1000,
        idToken: tokens.id_token ?? '',
        accessToken: tokens.access_token,
        refreshToken: tokens.refresh_token,
        claims: claims as Record<string, unknown>,
      };

      const sid = await sessions.create(record);
      // Set the Secure flag only when the public URL is HTTPS — on
      // plain HTTP (e.g. an appliance reached by raw IP on the LAN)
      // a Secure cookie would be silently dropped by the browser.
      const isHttps = env.API_PUBLIC_URL.startsWith('https://');
      reply.setCookie(env.SESSION_COOKIE_NAME, sid, {
        httpOnly: true,
        sameSite: 'lax',
        secure: isHttps,
        path: '/',
        signed: true,
        maxAge: SESSION_TTL_SECONDS,
      });
      return reply.redirect(stored.redirectTo || '/');
    }
  );

  // -- POST /api/v1/auth/password-login ---------------------------------
  // Single-page login: SPA posts username + password; api uses the
  // OIDC Resource Owner Password Credentials grant against Keycloak,
  // creates a session on success, sets the cookie, and returns the
  // user info. The browser never leaves the SPA.
  app.post(
    '/api/v1/auth/password-login',
    {
      schema: {
        description: 'Username/password login (Keycloak ROPC grant).',
        tags: ['auth'],
        body: {
          type: 'object',
          required: ['username', 'password'],
          properties: {
            username: { type: 'string', minLength: 1, maxLength: 128 },
            password: { type: 'string', minLength: 1, maxLength: 256 },
          },
        },
      },
    },
    async (req: FastifyRequest, reply: FastifyReply) => {
      const body = PasswordLoginBody.parse(req.body);
      let tokens;
      try {
        tokens = await keycloak.passwordLogin(body.username, body.password);
      } catch (err) {
        const status = (err as { status?: number }).status;
        // Map Keycloak's 400/401 → user-facing "invalid credentials".
        // Anything else is a server-side problem we shouldn't paper over.
        if (status === 400 || status === 401) {
          return reply.code(401).send({ error: 'invalid_credentials' });
        }
        req.log.error({ err }, 'password_login.upstream_failed');
        return reply.code(502).send({ error: 'auth_service_unavailable' });
      }

      const claims = tokens.claims();
      if (!claims) {
        return reply.code(500).send({ error: 'no_id_token_claims' });
      }
      const user = userFromClaims(claims as Record<string, unknown>);
      const now = Date.now();
      const record: SessionRecord = {
        userId: user.sub,
        username: user.username,
        createdAt: now,
        expiresAt: now + SESSION_TTL_SECONDS * 1000,
        idToken: tokens.id_token ?? '',
        accessToken: tokens.access_token,
        refreshToken: tokens.refresh_token,
        claims: claims as Record<string, unknown>,
      };
      const sid = await sessions.create(record);
      const isHttps = env.API_PUBLIC_URL.startsWith('https://');
      reply.setCookie(env.SESSION_COOKIE_NAME, sid, {
        httpOnly: true,
        sameSite: 'lax',
        secure: isHttps,
        path: '/',
        signed: true,
        maxAge: SESSION_TTL_SECONDS,
      });
      return reply.send({ user });
    }
  );

  // -- POST /api/v1/auth/logout -----------------------------------------
  app.post(
    '/api/v1/auth/logout',
    { schema: { description: 'Destroy session.', tags: ['auth'] } },
    async (req: FastifyRequest, reply: FastifyReply) => {
      const sid = req.sessionId;
      if (sid) {
        const record = await sessions.get(sid);
        if (record?.refreshToken) {
          await keycloak.logout(record.refreshToken);
        }
        await sessions.destroy(sid);
      }
      reply.clearCookie(env.SESSION_COOKIE_NAME, { path: '/' });
      return reply.send({ ok: true });
    }
  );

  // -- POST /api/v1/auth/device-code ------------------------------------
  // Initiates an OIDC device authorization flow for CLI clients. The
  // Keycloak server handles the flow; we proxy the request so the CLI
  // doesn't need to know the Keycloak endpoint directly.
  app.post(
    '/api/v1/auth/device-code',
    {
      schema: {
        description: 'Start an OIDC device authorization flow (for CLI).',
        tags: ['auth'],
        response: {
          200: {
            type: 'object',
            properties: {
              device_code: { type: 'string' },
              user_code: { type: 'string' },
              verification_uri: { type: 'string' },
              verification_uri_complete: { type: 'string' },
              expires_in: { type: 'number' },
              interval: { type: 'number' },
            },
            required: ['device_code', 'user_code', 'verification_uri', 'expires_in'],
          },
        },
      },
    },
    async (_req: FastifyRequest, reply: FastifyReply) => {
      const url = new URL(
        `${env.KEYCLOAK_ISSUER_URL.replace(/\/$/, '')}/protocol/openid-connect/auth/device`
      );
      try {
        const body = new URLSearchParams({
          client_id: env.KEYCLOAK_CLIENT_ID,
          scope: 'openid profile email',
        });
        const res = await fetch(url, {
          method: 'POST',
          headers: { 'content-type': 'application/x-www-form-urlencoded' },
          body: body.toString(),
        });
        if (!res.ok) {
          return reply
            .code(502)
            .send({ error: 'upstream_error', message: `device flow init failed: ${res.status}` });
        }
        const json = (await res.json()) as Record<string, unknown>;
        return reply.send(json);
      } catch (err) {
        return reply.code(502).send({
          error: 'upstream_error',
          message: (err as Error).message ?? 'device flow unreachable',
        });
      }
    }
  );

  // -- POST /api/v1/auth/token ------------------------------------------
  // CLI polls this endpoint with { device_code } until the user finishes
  // authenticating in the browser. We proxy to Keycloak's token endpoint.
  app.post<{ Body: { device_code?: string; grant_type?: string } }>(
    '/api/v1/auth/token',
    {
      schema: {
        description: 'Exchange a device_code for tokens (CLI polling endpoint).',
        tags: ['auth'],
        body: {
          type: 'object',
          properties: {
            device_code: { type: 'string' },
            grant_type: { type: 'string' },
          },
        },
      },
    },
    async (req, reply) => {
      const deviceCode = req.body?.device_code;
      if (!deviceCode) {
        return reply.code(400).send({ error: 'invalid_body', message: 'device_code required' });
      }
      const url = new URL(
        `${env.KEYCLOAK_ISSUER_URL.replace(/\/$/, '')}/protocol/openid-connect/token`
      );
      const body = new URLSearchParams({
        grant_type: req.body?.grant_type ?? 'urn:ietf:params:oauth:grant-type:device_code',
        device_code: deviceCode,
        client_id: env.KEYCLOAK_CLIENT_ID,
      });
      try {
        const res = await fetch(url, {
          method: 'POST',
          headers: { 'content-type': 'application/x-www-form-urlencoded' },
          body: body.toString(),
        });
        const json = (await res.json()) as Record<string, unknown>;
        // Keycloak returns 400 with {error: "authorization_pending"} while
        // the user hasn't completed the flow yet — forward verbatim so the
        // CLI can back off.
        return reply.code(res.status).send(json);
      } catch (err) {
        return reply.code(502).send({
          error: 'upstream_error',
          message: (err as Error).message ?? 'token endpoint unreachable',
        });
      }
    }
  );

  // -- GET /api/v1/auth/me (alias for /api/v1/me) -----------------------
  app.get(
    '/api/v1/auth/me',
    {
      preHandler: requireAuth,
      schema: {
        description: 'Return the current authenticated user (CLI-facing alias).',
        tags: ['auth'],
        security: [{ sessionCookie: [] }],
      },
    },
    async (req: FastifyRequest, reply: FastifyReply) => {
      const u = req.user;
      if (!u) return reply.code(401).send({ error: 'unauthorized' });
      return reply.send({
        sub: u.sub,
        username: u.username,
        email: u.email,
        name: u.name,
        roles: u.roles,
        groups: u.groups,
        tenant: u.tenant,
      });
    }
  );

  // -- GET /api/v1/me ---------------------------------------------------
  app.get(
    '/api/v1/me',
    {
      preHandler: requireAuth,
      schema: {
        description: 'Return the current authenticated user.',
        tags: ['auth'],
        security: [{ sessionCookie: [] }],
        response: {
          200: {
            type: 'object',
            properties: {
              sub: { type: 'string' },
              username: { type: 'string' },
              email: { type: 'string' },
              name: { type: 'string' },
              roles: { type: 'array', items: { type: 'string' } },
              groups: { type: 'array', items: { type: 'string' } },
              tenant: { type: 'string' },
            },
            required: ['sub', 'username', 'roles', 'groups', 'tenant'],
          },
        },
      },
    },
    async (req: FastifyRequest, reply: FastifyReply) => {
      const u = req.user;
      if (!u) return reply.code(401).send({ error: 'unauthorized' });
      return reply.send({
        sub: u.sub,
        username: u.username,
        email: u.email,
        name: u.name,
        roles: u.roles,
        groups: u.groups,
        tenant: u.tenant,
      });
    }
  );
}
