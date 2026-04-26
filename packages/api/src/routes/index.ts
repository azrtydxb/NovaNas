import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import type { Redis } from 'ioredis';
import type { SessionStore } from '../auth/session.js';
import type { Env } from '../env.js';
import type { DbClient } from '../services/db.js';
import type { JobsService } from '../services/jobs.js';
import type { CertManagerClient } from '../services/cert-manager.js';
import type { KeycloakAdmin } from '../services/keycloak-admin.js';
import type { KeycloakClient } from '../services/keycloak.js';
import type { OpenBaoAdmin } from '../services/openbao-admin.js';
import type { PromClient } from '../services/prom.js';
import type { WsHub } from '../ws/hub.js';

import { alertChannelsRoutes } from './alert-channels.js';
import { alertPoliciesRoutes } from './alert-policies.js';
import { apiTokensRoutes } from './api-tokens.js';
import { appCatalogRoutes } from './app-catalogs.js';
import { appsAvailableRoutes } from './apps-available.js';
import { appRoutes } from './apps.js';
import { auditPolicyRoutes } from './audit-policy.js';
import { auditRoutes } from './audit.js';
import { authRoutes } from './auth.js';
import { bondsRoutes } from './bonds.js';
import { bucketUserRoutes } from './bucket-users.js';
import { bucketRoutes } from './buckets.js';
import { certificatesRoutes } from './certificates.js';
import { cloudBackupJobsRoutes } from './cloud-backup-jobs.js';
import { cloudBackupTargetsRoutes } from './cloud-backup-targets.js';
import { clusterNetworkRoutes } from './cluster-network.js';
import { compositeRoutes } from './composite.js';
import { configBackupPolicyRoutes } from './config-backup-policy.js';
import { customDomainsRoutes } from './custom-domains.js';
import { datasetRoutes } from './datasets.js';
import { diskRoutes } from './disks.js';
import { encryptionPoliciesRoutes } from './encryption-policies.js';
import { firewallRulesRoutes } from './firewall-rules.js';
import { gpuDevicesRoutes } from './gpu-devices.js';
import { groupsRoutes } from './groups.js';
import { healthRoutes } from './health.js';
import { hostInterfacesRoutes } from './host-interfaces.js';
import { ingressesRoutes } from './ingresses.js';
import { iscsiTargetRoutes } from './iscsi-targets.js';
import { isoLibraryRoutes } from './iso-libraries.js';
import { jobsRoutes } from './jobs.js';
import { keycloakRealmsRoutes } from './keycloak-realms.js';
import { kmsKeysRoutes } from './kms-keys.js';
import { metricsRoutes } from './metrics.js';
import { nfsServerRoutes } from './nfs-servers.js';
import { nvmeofTargetRoutes } from './nvmeof-targets.js';
import { objectStoreRoutes } from './object-stores.js';
import { physicalInterfacesRoutes } from './physical-interfaces.js';
import { poolRoutes } from './pools.js';
import { remoteAccessTunnelsRoutes } from './remote-access-tunnels.js';
import { replicationJobsRoutes } from './replication-jobs.js';
import { replicationTargetsRoutes } from './replication-targets.js';
import { scrubSchedulesRoutes } from './scrub-schedules.js';
import { searchRoutes } from './search.js';
import { servicePolicyRoutes } from './service-policy.js';
import { shareRoutes } from './shares.js';
import { slosRoutes } from './slos.js';
import { smartPoliciesRoutes } from './smart-policies.js';
import { smbServerRoutes } from './smb-servers.js';
import { snapshotSchedulesRoutes } from './snapshot-schedules.js';
import { snapshotRoutes } from './snapshots.js';
import { sshKeysRoutes } from './ssh-keys.js';
import { systemSettingsRoutes } from './system-settings.js';
import { systemRoutes } from './system.js';
import { trafficPoliciesRoutes } from './traffic-policies.js';
import { updatePolicyRoutes } from './update-policy.js';
import { upsPolicyRoutes } from './ups-policy.js';
import { userRoutes } from './users.js';
import { versionRoutes } from './version.js';
import { vipPoolsRoutes } from './vip-pools.js';
import { vlansRoutes } from './vlans.js';
import { vmConsoleRoutes } from './vm-console.js';
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
  /** Keycloak Admin REST client for inlined operator side effects (#51). */
  keycloakAdmin?: KeycloakAdmin | null;
  /** OpenBao admin client for inlined operator side effects (#51). */
  openbao?: OpenBaoAdmin | null;
  /** cert-manager projection client for Certificate hooks (#51). */
  certManager?: CertManagerClient | null;
}

export async function registerRoutes(app: FastifyInstance, deps: RouteDeps): Promise<void> {
  // unauthenticated
  await app.register(async (s) => healthRoutes(s, { redis: deps.redis, db: deps.db ?? null }));
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
  // Postgres-backed (post-CRD-migration): pools + disks read/write
  // through PgResource. The kubeCustom client is no longer the source
  // of truth for these kinds.
  // -----------------------------------------------------------------
  // Postgres-backed resources (CRD-to-Postgres migration). Routes
  // take a DbClient and use PgResource. Validation is enforced at
  // the API layer; there is no kube-apiserver fallback.
  // -----------------------------------------------------------------
  const db = deps.db ?? null;
  await app.register(async (s) => poolRoutes(s, db));
  await app.register(async (s) => datasetRoutes(s, db, deps.openbao ?? null));
  await app.register(async (s) => bucketRoutes(s, db));
  await app.register(async (s) => diskRoutes(s, db));
  await app.register(async (s) => snapshotRoutes(s, db, { jobs: deps.jobs ?? null }));
  await app.register(async (s) => userRoutes(s, db, deps.keycloakAdmin ?? null));
  await app.register(async (s) => bucketUserRoutes(s, db));
  await app.register(async (s) => appCatalogRoutes(s, db));
  await app.register(async (s) => isoLibraryRoutes(s, db));
  await app.register(async (s) => encryptionPoliciesRoutes(s, db));
  await app.register(async (s) => kmsKeysRoutes(s, db, deps.openbao ?? null));
  await app.register(async (s) =>
    certificatesRoutes(s, db, deps.certManager ?? null, deps.env.SYSTEM_NAMESPACE)
  );
  await app.register(async (s) => snapshotSchedulesRoutes(s, db));
  await app.register(async (s) => replicationTargetsRoutes(s, db));
  await app.register(async (s) => replicationJobsRoutes(s, db));
  await app.register(async (s) => cloudBackupTargetsRoutes(s, db));
  await app.register(async (s) => cloudBackupJobsRoutes(s, db));
  await app.register(async (s) => scrubSchedulesRoutes(s, db));
  await app.register(async (s) => smartPoliciesRoutes(s, db));
  await app.register(async (s) => alertChannelsRoutes(s, db));
  await app.register(async (s) => alertPoliciesRoutes(s, db));
  await app.register(async (s) => auditPolicyRoutes(s, db));
  await app.register(async (s) => upsPolicyRoutes(s, db));
  await app.register(async (s) => slosRoutes(s, db));
  await app.register(async (s) => configBackupPolicyRoutes(s, db));
  await app.register(async (s) => systemSettingsRoutes(s, db));
  await app.register(async (s) => updatePolicyRoutes(s, db));
  await app.register(async (s) => groupsRoutes(s, db, deps.keycloakAdmin ?? null));
  await app.register(async (s) => keycloakRealmsRoutes(s, db, deps.keycloakAdmin ?? null));
  await app.register(async (s) => apiTokensRoutes(s, db));
  // Grey-set resources flipped via Option B: source of truth in
  // Postgres; host agents (nmstate, sshd, samba/knfsd, keepalived,
  // s3gw, iscsi/nvmeof targets) become API clients via TokenReview.
  // Tracked separately: host-agent rewrite issue.
  await app.register(async (s) => shareRoutes(s, db));
  await app.register(async (s) => sshKeysRoutes(s, db));
  await app.register(async (s) => bondsRoutes(s, db));
  await app.register(async (s) => vlansRoutes(s, db));
  await app.register(async (s) => physicalInterfacesRoutes(s, db));
  await app.register(async (s) => hostInterfacesRoutes(s, db));
  await app.register(async (s) => clusterNetworkRoutes(s, db));
  await app.register(async (s) => vipPoolsRoutes(s, db));
  await app.register(async (s) => customDomainsRoutes(s, db));
  await app.register(async (s) => objectStoreRoutes(s, db));
  await app.register(async (s) => iscsiTargetRoutes(s, db));
  await app.register(async (s) => nvmeofTargetRoutes(s, db));
  // nfs-ganesha and samba run on the host (no Pod-per-server model);
  // API just holds the config, host agent renders ganesha.conf /
  // smb.conf and reloads. Tracked under #55.
  await app.register(async (s) => nfsServerRoutes(s, db));
  await app.register(async (s) => smbServerRoutes(s, db));

  // -----------------------------------------------------------------
  // CRD-backed resources still on the K8s control path. These are
  // workload-producing (Vm via KubeVirt, AppInstance via Helm,
  // NfsServer/SmbServer Pods) or project to external CRDs
  // (novanet/novaedge for Ingress, FirewallRule, TrafficPolicy,
  // ServicePolicy, RemoteAccessTunnel). GpuDevice surfaces device-
  // plugin state. apps-available is the AppCatalog-synthesised
  // read-only view.
  // -----------------------------------------------------------------
  await app.register(async (s) => appRoutes(s, deps.kubeCustom));
  await app.register(async (s) => appsAvailableRoutes(s, deps.kubeCustom));
  await app.register(async (s) => vmRoutes(s, deps.kubeCustom));
  await app.register(async (s) => ingressesRoutes(s, deps.kubeCustom));
  await app.register(async (s) => remoteAccessTunnelsRoutes(s, deps.kubeCustom));
  await app.register(async (s) => firewallRulesRoutes(s, deps.kubeCustom));
  await app.register(async (s) => trafficPoliciesRoutes(s, deps.kubeCustom));
  await app.register(async (s) => servicePolicyRoutes(s, deps.kubeCustom));
  await app.register(async (s) => gpuDevicesRoutes(s, deps.kubeCustom));

  // B3-API-Infra: cross-resource search
  await app.register(async (s) =>
    searchRoutes(s, { kubeCustom: deps.kubeCustom, db: deps.db ?? null, redis: deps.redis })
  );

  await app.register(async (s) => systemRoutes(s, { jobs: deps.jobs ?? null }));

  // infra routes (audit, jobs, metrics) — A10-API-Infra
  await app.register(async (s) => auditRoutes(s, { db: deps.db ?? null }));
  await app.register(async (s) => jobsRoutes(s, { jobs: deps.jobs ?? null }));
  await app.register(async (s) => metricsRoutes(s, { prom: deps.prom ?? null }));

  // A11-Composite-SPICE: multi-CRD composite ops + VM console WS proxy
  await app.register(async (s) =>
    compositeRoutes(s, {
      kubeCustom: deps.kubeCustom,
      db: deps.db ?? null,
      jobs: deps.jobs ?? null,
    })
  );
  await app.register(async (s) => vmConsoleRoutes(s, { env: deps.env, sessions: deps.sessions }));

  // websocket
  await app.register(async (s) =>
    wsRoutes(s, { env: deps.env, sessions: deps.sessions, hub: deps.hub })
  );
}
