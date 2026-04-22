import Fastify, { type FastifyInstance } from 'fastify';
import type { Redis } from 'ioredis';
import type { Logger } from 'pino';
import { Registry, collectDefaultMetrics } from 'prom-client';
import type { Env } from './env.js';
import { registerCors } from './plugins/cors.js';
import { registerHelmet } from './plugins/helmet.js';
import { registerCookie } from './plugins/cookie.js';
import { registerRateLimit } from './plugins/rate-limit.js';
import { registerWebsocket } from './plugins/websocket.js';
import { registerSwagger } from './plugins/swagger.js';
import { registerAuth } from './plugins/auth.js';
import { registerRoutes } from './routes/index.js';
import { SessionStore } from './auth/session.js';
import type { KeycloakClient } from './services/keycloak.js';
import { WsHub } from './ws/hub.js';
import { PubSub } from './ws/pubsub.js';

export interface BuildAppOptions {
  env: Env;
  logger: Logger;
  redis: Redis;
  redisSub?: Redis; // optional dedicated sub connection
  keycloak: KeycloakClient;
  /** Disable optional subsystems in tests. */
  disableSwagger?: boolean;
  disablePubSub?: boolean;
  /** Custom metrics registry (default: new Registry per app). */
  metricsRegistry?: Registry;
}

export interface BuiltApp {
  app: FastifyInstance;
  hub: WsHub;
  sessions: SessionStore;
  pubsub?: PubSub;
  metricsRegistry: Registry;
}

/**
 * Build a fully configured Fastify app.
 *
 * Keep this factory free of side-effects beyond what the tests need;
 * network connections (DB, Redis, Keycloak) are constructed by `index.ts`
 * and injected here so unit tests can inject mocks.
 */
export async function buildApp(opts: BuildAppOptions): Promise<BuiltApp> {
  const {
    env,
    logger,
    redis,
    redisSub,
    keycloak,
    disableSwagger = false,
    disablePubSub = false,
  } = opts;

  const app = Fastify({
    logger,
    trustProxy: true,
    disableRequestLogging: false,
    bodyLimit: 5 * 1024 * 1024, // 5 MiB
  });

  // plugins
  await registerHelmet(app);
  await registerCors(app);
  await registerCookie(app, env);
  await registerRateLimit(app);
  await registerWebsocket(app);
  if (!disableSwagger) await registerSwagger(app, env);

  // session + auth
  const sessions = new SessionStore(redis);
  await registerAuth(app, env, sessions);

  // websocket hub + pubsub
  const hub = new WsHub();
  let pubsub: PubSub | undefined;
  if (!disablePubSub && redisSub) {
    pubsub = new PubSub(redisSub, redis, hub, logger);
    await pubsub.start();
  }

  // metrics
  const metricsRegistry = opts.metricsRegistry ?? new Registry();
  collectDefaultMetrics({ register: metricsRegistry, prefix: 'novanas_api_' });
  app.get('/metrics', { logLevel: 'warn' }, async (_req, reply) => {
    reply.header('content-type', metricsRegistry.contentType);
    return metricsRegistry.metrics();
  });

  // routes
  await registerRoutes(app, { env, redis, keycloak, sessions, hub });

  // 404 fallthrough
  app.setNotFoundHandler((_req, reply) => {
    reply.code(404).send({ error: 'not_found' });
  });

  // Centralised error handler
  app.setErrorHandler((err, req, reply) => {
    req.log.error({ err }, 'request.error');
    const statusCode = err.statusCode ?? 500;
    reply.code(statusCode).send({
      error: statusCode >= 500 ? 'internal_error' : err.name,
      message: statusCode >= 500 ? 'internal server error' : err.message,
    });
  });

  return { app, hub, sessions, pubsub, metricsRegistry };
}
