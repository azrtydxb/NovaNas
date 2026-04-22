import type { FastifyInstance } from 'fastify';
import type { Redis } from 'ioredis';
import type { SessionStore } from '../auth/session.js';
import type { Env } from '../env.js';
import type { KeycloakClient } from '../services/keycloak.js';
import type { WsHub } from '../ws/hub.js';

import { appRoutes } from './apps.js';
import { authRoutes } from './auth.js';
import { bucketRoutes } from './buckets.js';
import { datasetRoutes } from './datasets.js';
import { diskRoutes } from './disks.js';
import { healthRoutes } from './health.js';
import { poolRoutes } from './pools.js';
import { shareRoutes } from './shares.js';
import { snapshotRoutes } from './snapshots.js';
import { systemRoutes } from './system.js';
import { userRoutes } from './users.js';
import { versionRoutes } from './version.js';
import { vmRoutes } from './vms.js';
import { wsRoutes } from './ws.js';

export interface RouteDeps {
  env: Env;
  redis: Redis;
  keycloak: KeycloakClient;
  sessions: SessionStore;
  hub: WsHub;
}

export async function registerRoutes(app: FastifyInstance, deps: RouteDeps): Promise<void> {
  // unauthenticated
  await app.register(healthRoutes);
  await app.register(async (s) => versionRoutes(s, deps.env));

  // auth flow (some public, some require session)
  await app.register(async (s) =>
    authRoutes(s, {
      env: deps.env,
      keycloak: deps.keycloak,
      sessions: deps.sessions,
      redis: deps.redis,
    })
  );

  // domain stubs (all require session)
  await app.register(poolRoutes);
  await app.register(datasetRoutes);
  await app.register(bucketRoutes);
  await app.register(shareRoutes);
  await app.register(diskRoutes);
  await app.register(snapshotRoutes);
  await app.register(appRoutes);
  await app.register(vmRoutes);
  await app.register(userRoutes);
  await app.register(systemRoutes);

  // websocket
  await app.register(async (s) =>
    wsRoutes(s, { env: deps.env, sessions: deps.sessions, hub: deps.hub })
  );
}
