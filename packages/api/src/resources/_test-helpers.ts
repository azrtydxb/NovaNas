import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { Redis } from 'ioredis';
import { type BuiltApp, buildApp } from '../app.js';
import type { Env } from '../env.js';
import type { DbClient } from '../services/db.js';
import type { KeycloakClient } from '../services/keycloak.js';
import type { AuthenticatedUser } from '../types.js';

/** RFC 7396 JSON merge-patch — good enough for our fake. */
function mergePatch(
  target: Record<string, unknown>,
  patch: Record<string, unknown>
): Record<string, unknown> {
  const out: Record<string, unknown> = { ...target };
  for (const [k, v] of Object.entries(patch)) {
    if (v === null) {
      delete out[k];
    } else if (
      typeof v === 'object' &&
      !Array.isArray(v) &&
      typeof out[k] === 'object' &&
      out[k] !== null &&
      !Array.isArray(out[k])
    ) {
      out[k] = mergePatch(out[k] as Record<string, unknown>, v as Record<string, unknown>);
    } else {
      out[k] = v;
    }
  }
  return out;
}

/** An in-memory fake of the subset of `CustomObjectsApi` the API server uses. */
export class FakeCustomObjectsApi {
  private cluster = new Map<string, Map<string, Record<string, unknown>>>();
  private nsStore = new Map<string, Map<string, Map<string, Record<string, unknown>>>>();

  private clusterBucket(plural: string): Map<string, Record<string, unknown>> {
    let b = this.cluster.get(plural);
    if (!b) {
      b = new Map();
      this.cluster.set(plural, b);
    }
    return b;
  }
  private nsBucket(ns: string, plural: string): Map<string, Record<string, unknown>> {
    let nsMap = this.nsStore.get(ns);
    if (!nsMap) {
      nsMap = new Map();
      this.nsStore.set(ns, nsMap);
    }
    let b = nsMap.get(plural);
    if (!b) {
      b = new Map();
      nsMap.set(plural, b);
    }
    return b;
  }

  /**
   * Seed a resource for a test. After the CRD-to-Postgres migration,
   * migrated kinds (Disk, StoragePool, Dataset, …) are stored in
   * pglite via the polymorphic `resources` table. Non-migrated kinds
   * (Vm, AppInstance, network projections, etc.) still use the
   * in-memory map. The plural→kind+migrated lookup is in
   * MIGRATED_PLURALS at the bottom of this file.
   *
   * For migrated kinds we go through the same drizzle binding the
   * test app uses — using a separate connection (or a different
   * drizzle-orm install) would create a row that the route's
   * PgResource cannot see, since pnpm produces distinct module
   * instances under different peer-dep variants.
   */
  async seed(plural: string, obj: Record<string, unknown>, namespace?: string): Promise<void> {
    const migrated = MIGRATED_PLURALS[plural];
    if (migrated && this.dbInsert) {
      const meta = (obj.metadata ?? {}) as {
        name?: string;
        namespace?: string;
        labels?: Record<string, string>;
        annotations?: Record<string, string>;
      };
      const name = meta.name ?? '';
      const ns = namespace ?? meta.namespace ?? '';
      await this.dbInsert(migrated, name, ns, {
        labels: meta.labels ?? {},
        annotations: meta.annotations ?? {},
        spec: (obj.spec ?? {}) as Record<string, unknown>,
        status: (obj.status ?? {}) as Record<string, unknown>,
      });
      return;
    }
    const name = (obj.metadata as { name?: string } | undefined)?.name ?? '';
    const bucket = namespace ? this.nsBucket(namespace, plural) : this.clusterBucket(plural);
    bucket.set(name, obj);
  }

  /** Inserter for migrated kinds. Provided by buildTestApp. */
  dbInsert?: (
    kind: string,
    name: string,
    namespace: string,
    body: {
      labels: Record<string, string>;
      annotations: Record<string, string>;
      spec: Record<string, unknown>;
      status: Record<string, unknown>;
    }
  ) => Promise<void>;

  // cluster-scoped
  async listClusterCustomObject(
    _g: string,
    _v: string,
    plural: string
  ): Promise<{ response: unknown; body: unknown }> {
    return {
      response: {},
      body: { items: Array.from(this.clusterBucket(plural).values()) },
    };
  }
  async getClusterCustomObject(
    _g: string,
    _v: string,
    plural: string,
    name: string
  ): Promise<{ response: unknown; body: unknown }> {
    const obj = this.clusterBucket(plural).get(name);
    if (!obj) throw Object.assign(new Error('not found'), { statusCode: 404 });
    return { response: {}, body: obj };
  }
  async createClusterCustomObject(
    _g: string,
    _v: string,
    plural: string,
    body: object
  ): Promise<{ response: unknown; body: unknown }> {
    const obj = body as Record<string, unknown>;
    const name = (obj.metadata as { name?: string } | undefined)?.name ?? '';
    const bucket = this.clusterBucket(plural);
    if (bucket.has(name)) throw Object.assign(new Error('conflict'), { statusCode: 409 });
    bucket.set(name, obj);
    return { response: {}, body: obj };
  }
  async patchClusterCustomObject(
    _g: string,
    _v: string,
    plural: string,
    name: string,
    patch: object
  ): Promise<{ response: unknown; body: unknown }> {
    const bucket = this.clusterBucket(plural);
    const cur = bucket.get(name);
    if (!cur) throw Object.assign(new Error('not found'), { statusCode: 404 });
    const merged = mergePatch(cur, patch as Record<string, unknown>);
    bucket.set(name, merged);
    return { response: {}, body: merged };
  }
  async deleteClusterCustomObject(
    _g: string,
    _v: string,
    plural: string,
    name: string
  ): Promise<{ response: unknown; body: unknown }> {
    const bucket = this.clusterBucket(plural);
    if (!bucket.has(name)) throw Object.assign(new Error('not found'), { statusCode: 404 });
    bucket.delete(name);
    return { response: {}, body: {} };
  }

  // namespaced
  async listNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string
  ): Promise<{ response: unknown; body: unknown }> {
    return {
      response: {},
      body: { items: Array.from(this.nsBucket(ns, plural).values()) },
    };
  }
  async getNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string,
    name: string
  ): Promise<{ response: unknown; body: unknown }> {
    const obj = this.nsBucket(ns, plural).get(name);
    if (!obj) throw Object.assign(new Error('not found'), { statusCode: 404 });
    return { response: {}, body: obj };
  }
  async createNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string,
    body: object
  ): Promise<{ response: unknown; body: unknown }> {
    const obj = body as Record<string, unknown>;
    const name = (obj.metadata as { name?: string } | undefined)?.name ?? '';
    const bucket = this.nsBucket(ns, plural);
    if (bucket.has(name)) throw Object.assign(new Error('conflict'), { statusCode: 409 });
    bucket.set(name, obj);
    return { response: {}, body: obj };
  }
  async patchNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string,
    name: string,
    patch: object
  ): Promise<{ response: unknown; body: unknown }> {
    const bucket = this.nsBucket(ns, plural);
    const cur = bucket.get(name);
    if (!cur) throw Object.assign(new Error('not found'), { statusCode: 404 });
    const merged = mergePatch(cur, patch as Record<string, unknown>);
    bucket.set(name, merged);
    return { response: {}, body: merged };
  }
  async deleteNamespacedCustomObject(
    _g: string,
    _v: string,
    ns: string,
    plural: string,
    name: string
  ): Promise<{ response: unknown; body: unknown }> {
    const bucket = this.nsBucket(ns, plural);
    if (!bucket.has(name)) throw Object.assign(new Error('not found'), { statusCode: 404 });
    bucket.delete(name);
    return { response: {}, body: {} };
  }
}

export function fakeRedis(): Redis {
  const store = new Map<string, string>();
  const api = {
    async get(k: string) {
      return store.get(k) ?? null;
    },
    async set(k: string, v: string) {
      store.set(k, v);
      return 'OK';
    },
    async setex(k: string, _ttl: number, v: string) {
      store.set(k, v);
      return 'OK';
    },
    async del(k: string) {
      return store.delete(k) ? 1 : 0;
    },
    async expire() {
      return 1;
    },
    async ping() {
      return 'PONG';
    },
    async publish() {
      return 0;
    },
    async psubscribe() {
      return 1;
    },
    async punsubscribe() {
      return 1;
    },
    on() {
      return api;
    },
    disconnect() {
      /* no-op */
    },
  };
  return api as unknown as Redis;
}

export function fakeKeycloak(): KeycloakClient {
  return {
    config: {} as never,
    issuerUrl: 'http://localhost/realms/test',
    clientId: 'test',
    async buildAuthUrl(_redirectUri: string) {
      return {
        url: new URL('http://localhost/authorize'),
        state: 's',
        nonce: 'n',
        codeVerifier: 'v',
      };
    },
    async exchangeCode() {
      throw new Error('not used in this test');
    },
    async passwordLogin() {
      throw new Error('not used in this test');
    },
    async logout() {
      /* no-op */
    },
  };
}

export const testEnv: Env = Object.freeze({
  NODE_ENV: 'test',
  PORT: 0,
  LOG_LEVEL: 'silent' as unknown as Env['LOG_LEVEL'],
  DATABASE_URL: 'postgres://localhost/test',
  REDIS_URL: 'redis://localhost:6379',
  KEYCLOAK_ISSUER_URL: 'http://localhost/realms/test',
  KEYCLOAK_CLIENT_ID: 'test',
  KEYCLOAK_CLIENT_SECRET: 'test-secret',
  SESSION_COOKIE_NAME: 'novanas_session',
  SESSION_SECRET: 'test-secret-0123456789abcdef',
  API_VERSION: '0.0.0-test',
  API_PUBLIC_URL: 'http://localhost:8080',
  KUBECONFIG_PATH: undefined,
  OPENBAO_ADDR: undefined,
  OPENBAO_TOKEN: undefined,
  PROMETHEUS_URL: undefined,
  SYSTEM_NAMESPACE: 'novanas-system',
});

export interface TestAppHandle {
  built: BuiltApp;
  kube: FakeCustomObjectsApi;
  /** Inject a user into every subsequent request by stashing their id under a test cookie. */
  authAs(user: Partial<AuthenticatedUser> & { username: string; roles: string[] }): Promise<string>;
}

/**
 * Build a Fastify app wired to a FakeCustomObjectsApi and return helpers for
 * creating authenticated test sessions.
 */
export async function buildTestApp(): Promise<TestAppHandle> {
  const kube = new FakeCustomObjectsApi();
  const redis = fakeRedis();
  // Fastify v5 splits the logger config: a config object goes in
  // `logger`, a pre-built pino instance in `loggerInstance`. Tests need
  // the latter. Use a real pino in silent mode rather than a typed-as-
  // never POJO that doesn't satisfy the BaseLogger interface.
  const { default: pino } = await import('pino');

  // In-memory Postgres for the migrated PgResource routes. pglite ships
  // a wasm Postgres; drizzle-orm/pglite is a thin adapter. We cast to
  // the postgres-js DbClient type because every call PgResource makes
  // is structurally compatible — Drizzle's query builder is the same
  // surface across adapters at runtime, just typed differently.
  const { PGlite } = await import('@electric-sql/pglite');
  const { drizzle: drizzlePglite } = await import('drizzle-orm/pglite');
  const dbSchema = await import('@novanas/db');
  const pgClient = new PGlite();
  const db = drizzlePglite(pgClient, { schema: dbSchema }) as unknown as DbClient;
  // Use raw pglite query for seeding — going through drizzle here
  // would resolve through a peer-dep-duplicated copy of drizzle-orm
  // (one with pglite peer, one without) and the table reference
  // wouldn't unify with what PgResource later reads through.
  kube.dbInsert = async (kind, name, namespace, body) => {
    await pgClient.query(
      `INSERT INTO resources (kind, name, namespace, labels, annotations, spec, status, revision)
       VALUES ($1, $2, $3, $4::jsonb, $5::jsonb, $6::jsonb, $7::jsonb, '1')
       ON CONFLICT (kind, namespace, name) DO UPDATE
       SET labels = EXCLUDED.labels,
           annotations = EXCLUDED.annotations,
           spec = EXCLUDED.spec,
           status = EXCLUDED.status,
           revision = (resources.revision::int + 1)::text,
           updated_at = now()`,
      [
        kind,
        name,
        namespace,
        JSON.stringify(body.labels),
        JSON.stringify(body.annotations),
        JSON.stringify(body.spec),
        JSON.stringify(body.status),
      ]
    );
  };
  // Apply the resources-table migration (the one PgResource needs).
  await pgClient.exec(`
    CREATE TABLE IF NOT EXISTS resources (
      kind        varchar(64)  NOT NULL,
      name        varchar(253) NOT NULL,
      namespace   varchar(253) NOT NULL DEFAULT '',
      labels      jsonb        NOT NULL DEFAULT '{}'::jsonb,
      annotations jsonb        NOT NULL DEFAULT '{}'::jsonb,
      spec        jsonb        NOT NULL DEFAULT '{}'::jsonb,
      status      jsonb        NOT NULL DEFAULT '{}'::jsonb,
      revision    text         NOT NULL DEFAULT '1',
      created_at  timestamptz  NOT NULL DEFAULT now(),
      updated_at  timestamptz  NOT NULL DEFAULT now(),
      deleted_at  timestamptz
    );
    CREATE UNIQUE INDEX IF NOT EXISTS resources_kind_namespace_name_idx
      ON resources (kind, namespace, name);
    CREATE INDEX IF NOT EXISTS resources_kind_idx ON resources (kind);
    CREATE INDEX IF NOT EXISTS resources_updated_idx ON resources (updated_at);
  `);

  const built = await buildApp({
    env: testEnv,
    logger: pino({ level: 'silent' }),
    redis,
    keycloak: fakeKeycloak(),
    kubeCustom: kube as unknown as CustomObjectsApi,
    db,
    disableSwagger: true,
    disablePubSub: true,
    disableScheduledTasks: true,
  });

  async function authAs(user: Partial<AuthenticatedUser> & { username: string; roles: string[] }) {
    const claims: Record<string, unknown> = {
      sub: user.sub ?? user.username,
      preferred_username: user.username,
      email: user.email,
      name: user.name,
      realm_access: { roles: user.roles },
      groups: user.groups ?? [],
    };
    const sid = await built.sessions.create({
      userId: user.sub ?? user.username,
      username: user.username,
      createdAt: Date.now(),
      expiresAt: Date.now() + 3600_000,
      idToken: 'test',
      accessToken: 'test',
      claims,
    });
    return sid;
  }

  return { built, kube, authAs };
}

/** Build a cookie header for the given session id, using the fastify signer. */
export function cookieFor(built: BuiltApp, sid: string): string {
  const signed = built.app.signCookie(sid);
  return `${testEnv.SESSION_COOKIE_NAME}=${encodeURIComponent(signed)}`;
}

/**
 * Plural → Kind for resources that have moved to Postgres. Used by
 * FakeCustomObjectsApi.seed() to route writes to the right backend.
 * Mirror of the migration commit list — when a new resource flips
 * onto PgResource, add an entry here so its tests keep working.
 */
const MIGRATED_PLURALS: Record<string, string> = {
  storagepools: 'StoragePool',
  disks: 'Disk',
  datasets: 'Dataset',
  snapshots: 'Snapshot',
  snapshotschedules: 'SnapshotSchedule',
  scrubschedules: 'ScrubSchedule',
  smartpolicies: 'SmartPolicy',
  encryptionpolicies: 'EncryptionPolicy',
  users: 'User',
  groups: 'Group',
  apitokens: 'ApiToken',
  keycloakrealms: 'KeycloakRealm',
  kmskeys: 'KmsKey',
  certificates: 'Certificate',
  appcatalogs: 'AppCatalog',
  isolibraries: 'IsoLibrary',
  buckets: 'Bucket',
  bucketusers: 'BucketUser',
  systemsettings: 'SystemSettings',
  servicelevelobjectives: 'ServiceLevelObjective',
  alertpolicies: 'AlertPolicy',
  auditpolicies: 'AuditPolicy',
  configbackuppolicies: 'ConfigBackupPolicy',
  updatepolicies: 'UpdatePolicy',
  upspolicies: 'UpsPolicy',
  alertchannels: 'AlertChannel',
  cloudbackuptargets: 'CloudBackupTarget',
  cloudbackupjobs: 'CloudBackupJob',
  replicationtargets: 'ReplicationTarget',
  replicationjobs: 'ReplicationJob',
  // Grey-set (Option B):
  shares: 'Share',
  sshkeys: 'SshKey',
  bonds: 'Bond',
  vlans: 'Vlan',
  physicalinterfaces: 'PhysicalInterface',
  hostinterfaces: 'HostInterface',
  clusternetworks: 'ClusterNetwork',
  vippools: 'VipPool',
  customdomains: 'CustomDomain',
  objectstores: 'ObjectStore',
  iscsitargets: 'IscsiTarget',
  nvmeoftargets: 'NvmeofTarget',
  nfsservers: 'NfsServer',
  smbservers: 'SmbServer',
  gpudevices: 'GpuDevice',
  ingresses: 'Ingress',
  remoteaccesstunnels: 'RemoteAccessTunnel',
  apps: 'App',
  appinstances: 'AppInstance',
  vms: 'Vm',
};
