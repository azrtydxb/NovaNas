import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import type { Redis } from 'ioredis';
import type { SessionStore } from '../auth/session.js';
import type { Env } from '../env.js';
import type { DbClient } from '../services/db.js';
import type { JobsService } from '../services/jobs.js';
import type { KeycloakClient } from '../services/keycloak.js';
import type { PromClient } from '../services/prom.js';
import type { WsHub } from '../ws/hub.js';

import { appCatalogRoutes } from './app-catalogs.js';
import { appsAvailableRoutes } from './apps-available.js';
import { appRoutes } from './apps.js';
import { auditRoutes } from './audit.js';
import { authRoutes } from './auth.js';
import { bucketUserRoutes } from './bucket-users.js';
import { bucketRoutes } from './buckets.js';
import { datasetRoutes } from './datasets.js';
import { diskRoutes } from './disks.js';
import { healthRoutes } from './health.js';
import { iscsiTargetRoutes } from './iscsi-targets.js';
import { isoLibraryRoutes } from './iso-libraries.js';
import { jobsRoutes } from './jobs.js';
import { metricsRoutes } from './metrics.js';
import { nfsServerRoutes } from './nfs-servers.js';
import { nvmeofTargetRoutes } from './nvmeof-targets.js';
import { objectStoreRoutes } from './object-stores.js';
import { poolRoutes } from './pools.js';
import { shareRoutes } from './shares.js';
import { smbServerRoutes } from './smb-servers.js';
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
  /** Kubernetes custom-objects client. Required for the 8 CRUD routes. */
  kubeCustom?: CustomObjectsApi;
  /** Drizzle client for audit / jobs persistence. Optional in tests. */
  db?: DbClient | null;
  /** Jobs service (requires db). */
  jobs?: JobsService | null;
  /** Prometheus client for metrics gateway. */
  prom?: PromClient | null;
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

  // domain routes (all require session). The 8 CRUD modules use kubeCustom
  // when available; otherwise they fall back to 503 stubs so the app still
  // boots in test environments without a kubeconfig.
  await app.register(async (s) => poolRoutes(s, deps.kubeCustom));
  await app.register(async (s) => datasetRoutes(s, deps.kubeCustom));
  await app.register(async (s) => bucketRoutes(s, deps.kubeCustom));
  await app.register(async (s) => shareRoutes(s, deps.kubeCustom));
  await app.register(async (s) => diskRoutes(s, deps.kubeCustom));
  await app.register(async (s) => snapshotRoutes(s, deps.kubeCustom));
  await app.register(async (s) => appRoutes(s, deps.kubeCustom));
  await app.register(async (s) => userRoutes(s, deps.kubeCustom));

  // A10-API-More: 10 additional CRUD resources
  await app.register(async (s) => objectStoreRoutes(s, deps.kubeCustom));
  await app.register(async (s) => bucketUserRoutes(s, deps.kubeCustom));
  await app.register(async (s) => smbServerRoutes(s, deps.kubeCustom));
  await app.register(async (s) => nfsServerRoutes(s, deps.kubeCustom));
  await app.register(async (s) => iscsiTargetRoutes(s, deps.kubeCustom));
  await app.register(async (s) => nvmeofTargetRoutes(s, deps.kubeCustom));
  await app.register(async (s) => appCatalogRoutes(s, deps.kubeCustom));
  await app.register(async (s) => appsAvailableRoutes(s, deps.kubeCustom));
  await app.register(async (s) => vmRoutes(s, deps.kubeCustom));
  await app.register(async (s) => isoLibraryRoutes(s, deps.kubeCustom));

  await app.register(systemRoutes);

  // infra routes (audit, jobs, metrics) — A10-API-Infra
  await app.register(async (s) => auditRoutes(s, { db: deps.db ?? null }));
  await app.register(async (s) => jobsRoutes(s, { jobs: deps.jobs ?? null }));
  await app.register(async (s) => metricsRoutes(s, { prom: deps.prom ?? null }));

  // websocket
  await app.register(async (s) =>
    wsRoutes(s, { env: deps.env, sessions: deps.sessions, hub: deps.hub })
  );
}
