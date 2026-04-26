#!/usr/bin/env node
import { existsSync } from 'node:fs';
import { dirname, resolve } from 'node:path';
import { fileURLToPath } from 'node:url';
import { migrate as runMigrations } from '@novanas/db';
import { buildApp } from './app.js';
import { loadEnv } from './env.js';
import { createLogger } from './logger.js';
import { startKubeWatch } from './plugins/kube-watch.js';
import { createDbClient } from './services/db.js';
import { createKeycloakAdmin } from './services/keycloak-admin.js';
import { createKeycloakClient } from './services/keycloak.js';
import { createKubeClients } from './services/kube.js';
import { createPromClient } from './services/prom.js';
import { createRedisClient } from './services/redis.js';
import { initTelemetry, shutdownTelemetry } from './telemetry.js';

async function main(): Promise<void> {
  const env = loadEnv();
  initTelemetry('novanas-api');
  const logger = createLogger({
    level: env.LOG_LEVEL,
    pretty: env.NODE_ENV === 'development',
  });

  logger.info({ port: env.PORT, env: env.NODE_ENV }, 'novanas-api.starting');

  const redis = createRedisClient(env);
  const redisSub = createRedisClient(env);
  const keycloak = await createKeycloakClient(env);
  // Admin REST client for inlined operator side effects (#51). Uses
  // the novanas-api confidential client's service account; safe to
  // construct unconditionally since it doesn't open any connections
  // until the first hook fires.
  const keycloakAdmin = createKeycloakAdmin(env);
  const kube = createKubeClients(env);
  const db = await createDbClient(env).catch((err) => {
    logger.error({ err }, 'novanas-api.db.connect_failed');
    return null;
  });

  // Run pending Drizzle migrations before serving any request (#43).
  // Locations checked, in order:
  //   1. DB_MIGRATIONS_DIR env (escape hatch / tests)
  //   2. /app/migrations — production runtime layout (Dockerfile copies them here)
  //   3. <package-relative>/migrations — local dev / npm-run
  // Idempotent: drizzle tracks applied entries in its own bookkeeping
  // table. Failures are logged but don't kill the process — operator
  // intervention is preferable to a crash loop on transient db error.
  if (db) {
    const candidates = [
      process.env.DB_MIGRATIONS_DIR,
      '/app/migrations',
      resolve(dirname(fileURLToPath(import.meta.url)), '../../db/migrations'),
    ].filter((p): p is string => Boolean(p));
    const migrationsFolder = candidates.find((p) => existsSync(p));
    if (!migrationsFolder) {
      logger.warn({ candidates }, 'novanas-api.migrations.folder_not_found');
    } else {
      try {
        await runMigrations(db, { migrationsFolder });
        logger.info({ migrationsFolder }, 'novanas-api.migrations.applied');
      } catch (err) {
        logger.error({ err, migrationsFolder }, 'novanas-api.migrations.failed');
      }
    }
  }

  const prom = createPromClient(env, { redis });

  const { app, pubsub } = await buildApp({
    env,
    logger,
    redis,
    redisSub,
    keycloak,
    keycloakAdmin,
    kubeCustom: kube.custom,
    kubeAuthn: kube.authn,
    db,
    prom,
  });

  const watch = startKubeWatch({ config: kube.config, redis, logger });

  const shutdown = async (signal: string): Promise<void> => {
    logger.info({ signal }, 'novanas-api.shutdown');
    try {
      await app.close();
      await watch.stop();
      await pubsub?.stop();
      redis.disconnect();
      redisSub.disconnect();
      await shutdownTelemetry();
    } catch (err) {
      logger.error({ err }, 'novanas-api.shutdown.error');
    } finally {
      process.exit(0);
    }
  };

  process.on('SIGTERM', () => void shutdown('SIGTERM'));
  process.on('SIGINT', () => void shutdown('SIGINT'));
  process.on('unhandledRejection', (err) => logger.error({ err }, 'unhandledRejection'));
  process.on('uncaughtException', (err) => logger.error({ err }, 'uncaughtException'));

  try {
    await app.listen({ port: env.PORT, host: '0.0.0.0' });
  } catch (err) {
    logger.fatal({ err }, 'novanas-api.listen.failed');
    process.exit(1);
  }
}

main().catch((err) => {
  // eslint-disable-next-line no-console
  console.error('novanas-api crashed during bootstrap:', err);
  process.exit(1);
});
