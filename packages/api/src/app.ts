import type { CustomObjectsApi } from '@kubernetes/client-node';
import Fastify, { type FastifyInstance } from 'fastify';
import type { Redis } from 'ioredis';
import type { Logger } from 'pino';
import { Registry, collectDefaultMetrics } from 'prom-client';
import { SessionStore } from './auth/session.js';
import type { Env } from './env.js';
import { registerAuth } from './plugins/auth.js';
import { registerCookie } from './plugins/cookie.js';
import { registerCors } from './plugins/cors.js';
import { registerHelmet } from './plugins/helmet.js';
import { registerRateLimit } from './plugins/rate-limit.js';
import { registerSwagger } from './plugins/swagger.js';
import { registerWebsocket } from './plugins/websocket.js';
import { registerRoutes } from './routes/index.js';
import type { DbClient } from './services/db.js';
import { JobsService } from './services/jobs.js';
import type { KeycloakClient } from './services/keycloak.js';
import type { PromClient } from './services/prom.js';
import { WsHub } from './ws/hub.js';
import { PubSub } from './ws/pubsub.js';

export interface BuildAppOptions {
  env: Env;
  logger: Logger;
  redis: Redis;
  redisSub?: Redis; // optional dedicated sub connection
  keycloak: KeycloakClient;
  /** Kubernetes CustomObjects client for CRUD routes. Optional in tests. */
  kubeCustom?: CustomObjectsApi;
  /** Disable optional subsystems in tests. */
  disableSwagger?: boolean;
  disablePubSub?: boolean;
  /** Custom metrics registry (default: new Registry per app). */
  metricsRegistry?: Registry;
  /** Drizzle client for audit / jobs. Optional; routes degrade to 503 when absent. */
  db?: DbClient | null;
  /** Prometheus client for metrics gateway. Optional. */
  prom?: PromClient | null;
}

export interface BuiltApp {
  app: FastifyInstance;
  hub: WsHub;
  sessions: SessionStore;
  pubsub?: PubSub;
  metricsRegistry: Registry;
  jobs?: JobsService | null;
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

  // jobs service (only when db is provided)
  const jobsService = opts.db ? new JobsService(opts.db, redis) : null;

  // routes
  await registerRoutes(app, {
    env,
    redis,
    keycloak,
    sessions,
    hub,
    kubeCustom: opts.kubeCustom,
    db: opts.db ?? null,
    jobs: jobsService,
    prom: opts.prom ?? null,
  });

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

  return { app, hub, sessions, pubsub, metricsRegistry, jobs: jobsService };
}
