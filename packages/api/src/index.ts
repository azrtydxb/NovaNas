#!/usr/bin/env node
import { buildApp } from './app.js';
import { loadEnv } from './env.js';
import { createLogger } from './logger.js';
import { createKeycloakClient } from './services/keycloak.js';
import { createKubeClients } from './services/kube.js';
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
  const kube = createKubeClients(env);

  const { app, pubsub } = await buildApp({
    env,
    logger,
    redis,
    redisSub,
    keycloak,
    kubeCustom: kube.custom,
  });

  const shutdown = async (signal: string): Promise<void> => {
    logger.info({ signal }, 'novanas-api.shutdown');
    try {
      await app.close();
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
