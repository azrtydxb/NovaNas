import type { CustomObjectsApi } from '@kubernetes/client-node';
import type { FastifyInstance } from 'fastify';
import type { Redis } from 'ioredis';
import { type Kind, canRead, ownNamespace } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { buildAppCatalogResource } from '../resources/app-catalogs.js';
import { buildAppResource } from '../resources/apps-available.js';
import { buildAppInstanceResource } from '../resources/apps.js';
import { buildBucketUserResource } from '../resources/bucket-users.js';
import { buildBucketResource } from '../resources/buckets.js';
import { buildDatasetResource } from '../resources/datasets.js';
import { buildDiskResource } from '../resources/disks.js';
import { buildGroupResource } from '../resources/groups.js';
import { buildObjectStoreResource } from '../resources/object-stores.js';
import { buildPoolResource } from '../resources/pools.js';
import { buildShareResource } from '../resources/shares.js';
import { buildSnapshotResource } from '../resources/snapshots.js';
import { buildUserResource } from '../resources/users.js';
import { buildVmResource } from '../resources/vms.js';
import type { Resource } from '../services/resource.js';
import type { AuthenticatedUser } from '../types.js';

export interface SearchDeps {
  kubeCustom?: CustomObjectsApi;
  /** Drizzle client for Postgres-backed resources (pools, disks). */
  db?: import('../services/db.js').DbClient | null;
  redis?: Redis | null;
  /** Concurrency for the fan-out. Default 4. */
  concurrency?: number;
  /** Redis cache TTL in seconds. Default 10. */
  cacheTtlSeconds?: number;
}

interface Searchable {
  /** Response-field grouping key. */
  group: string;
  /** Kind used for authz. */
  kind: Kind;
  /** `true` if namespaced; resolve to user's ownNamespace for read scope. */
  namespaced: boolean;
  /** Build the resource and list items. */
  list: (user: AuthenticatedUser) => Promise<Array<Record<string, unknown>>>;
}

interface ResourceLike {
  metadata?: {
    name?: string;
    namespace?: string;
    labels?: Record<string, string>;
    annotations?: Record<string, string>;
  };
}

/** Bounded-concurrency fan-out. */
async function mapLimit<T, R>(
  items: T[],
  limit: number,
  fn: (item: T) => Promise<R>
): Promise<R[]> {
  const results: R[] = new Array(items.length);
  let idx = 0;
  const workers = Array.from({ length: Math.max(1, limit) }, async () => {
    while (true) {
      const i = idx++;
      if (i >= items.length) return;
      results[i] = await fn(items[i]!);
    }
  });
  await Promise.all(workers);
  return results;
}

function matches(obj: ResourceLike, qLower: string): boolean {
  const name = obj.metadata?.name;
  if (typeof name === 'string' && name.toLowerCase().includes(qLower)) return true;
  const labels = obj.metadata?.labels;
  if (labels) {
    for (const v of Object.values(labels)) {
      if (typeof v === 'string' && v.toLowerCase() === qLower) return true;
    }
  }
  const annotations = obj.metadata?.annotations;
  if (annotations) {
    for (const v of Object.values(annotations)) {
      if (typeof v === 'string' && v.toLowerCase() === qLower) return true;
    }
  }
  return false;
}

function listFromResource<T>(
  resource: Resource<T>,
  namespace?: string
): Promise<Array<Record<string, unknown>>> {
  return resource
    .list({ namespace })
    .then((r) => r.items as unknown as Array<Record<string, unknown>>)
    .catch(() => []);
}

export async function searchRoutes(app: FastifyInstance, deps: SearchDeps): Promise<void> {
  const security = [{ sessionCookie: [] }];
  const { kubeCustom, db, redis, concurrency = 4, cacheTtlSeconds = 10 } = deps;

  if (!kubeCustom || !db) {
    app.get(
      '/api/v1/search',
      { preHandler: requireAuth, schema: { tags: ['search'], security } },
      async (_req, reply) =>
        reply.code(503).send({ error: 'unavailable', message: 'kubernetes client not configured' })
    );
    return;
  }

  const pools = buildPoolResource(db);
  const datasets = buildDatasetResource(db);
  const buckets = buildBucketResource(db);
  const shares = buildShareResource(db);
  const disks = buildDiskResource(db);
  const appsAvailable = buildAppResource(kubeCustom);
  const appInstances = buildAppInstanceResource(kubeCustom);
  const vms = buildVmResource(kubeCustom);
  const users = buildUserResource(db);
  const objectStores = buildObjectStoreResource(db);
  const snapshots = buildSnapshotResource(db);
  const bucketUsers = buildBucketUserResource(db);
  const appCatalogs = buildAppCatalogResource(db);
  const groups = buildGroupResource(db);

  const searchables: Searchable[] = [
    {
      group: 'pools',
      kind: 'StoragePool',
      namespaced: false,
      list: () => listFromResource(pools),
    },
    {
      group: 'datasets',
      kind: 'Dataset',
      namespaced: false,
      list: () => listFromResource(datasets),
    },
    {
      group: 'buckets',
      kind: 'Bucket',
      namespaced: false,
      list: () => listFromResource(buckets),
    },
    {
      group: 'shares',
      kind: 'Share',
      namespaced: false,
      list: () => listFromResource(shares),
    },
    {
      group: 'disks',
      kind: 'Disk',
      namespaced: false,
      list: () => listFromResource(disks),
    },
    {
      group: 'apps-available',
      kind: 'App',
      namespaced: false,
      list: () => listFromResource(appsAvailable),
    },
    {
      group: 'app-instances',
      kind: 'AppInstance',
      namespaced: true,
      list: (user) => listFromResource(appInstances, ownNamespace(user)),
    },
    {
      group: 'vms',
      kind: 'Vm',
      namespaced: true,
      list: (user) => listFromResource(vms, ownNamespace(user)),
    },
    {
      group: 'users',
      kind: 'User',
      namespaced: false,
      list: () => listFromResource(users),
    },
    {
      group: 'groups',
      kind: 'Group',
      namespaced: false,
      list: () => listFromResource(groups),
    },
    {
      group: 'object-stores',
      kind: 'ObjectStore',
      namespaced: false,
      list: () => listFromResource(objectStores),
    },
    {
      group: 'snapshots',
      kind: 'Snapshot',
      namespaced: false,
      list: () => listFromResource(snapshots),
    },
    {
      group: 'bucket-users',
      kind: 'BucketUser',
      namespaced: false,
      list: () => listFromResource(bucketUsers),
    },
    {
      group: 'app-catalogs',
      kind: 'AppCatalog',
      namespaced: false,
      list: () => listFromResource(appCatalogs),
    },
  ];

  app.get<{ Querystring: { q?: string; limit?: string } }>(
    '/api/v1/search',
    {
      preHandler: requireAuth,
      schema: {
        summary: 'Cross-resource search',
        tags: ['search'],
        security,
        querystring: {
          type: 'object',
          properties: {
            q: { type: 'string', minLength: 1 },
            limit: { type: 'string' },
          },
          required: ['q'],
        },
      },
    },
    async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      const q = (req.query.q ?? '').trim();
      if (!q) {
        return reply.code(400).send({ error: 'invalid_query', message: 'q required' });
      }
      const limit = Math.max(1, Math.min(200, Number.parseInt(req.query.limit ?? '20', 10) || 20));
      const qLower = q.toLowerCase();

      const cacheKey = `novanas:search:${user.sub || user.username}:${qLower}:${limit}`;
      if (redis) {
        try {
          const hit = await redis.get(cacheKey);
          if (hit) {
            const parsed = JSON.parse(hit) as Record<string, unknown>;
            return reply.header('x-cache', 'hit').send(parsed);
          }
        } catch {
          // cache read best-effort
        }
      }

      // filter to only those kinds the user is allowed to read
      const allowed = searchables.filter((s) =>
        canRead(user, s.kind, s.namespaced ? ownNamespace(user) : undefined)
      );

      const results = await mapLimit(allowed, concurrency, async (s) => {
        const items = await s.list(user);
        const filtered = items.filter((it) => matches(it as ResourceLike, qLower)).slice(0, limit);
        return [s.group, filtered] as const;
      });

      const grouped: Record<string, unknown[]> = {};
      for (const s of searchables) grouped[s.group] = [];
      for (const [group, items] of results) grouped[group] = items as unknown[];

      const body = { query: q, limit, results: grouped };

      if (redis) {
        try {
          await redis.setex(cacheKey, cacheTtlSeconds, JSON.stringify(body));
        } catch {
          // cache write best-effort
        }
      }

      return reply.header('x-cache', 'miss').send(body);
    }
  );
}
