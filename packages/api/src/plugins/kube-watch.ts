import { type KubeConfig, Watch } from '@kubernetes/client-node';
import type { FastifyBaseLogger } from 'fastify';
import type { Redis } from 'ioredis';

/**
 * Kubernetes watch plugin.
 *
 * Starts a long-lived watch per CRD kind and publishes every ADDED /
 * MODIFIED / DELETED event to Redis pub/sub under
 * `novanas:events:<channel>:<name>` (cluster-scoped) or
 * `novanas:events:<channel>:<namespace>/<name>` (namespaced).
 *
 * Events are deduplicated by `metadata.resourceVersion` on a per-object
 * basis; on stream break we reconnect using the most recently seen
 * resourceVersion for that kind.
 */

export interface WatchedKind {
  /** WS channel prefix (e.g. 'pool'). */
  channel: string;
  group: string;
  version: string;
  plural: string;
  /** Undefined for cluster-scoped. */
  namespaced: boolean;
}

/** The 18 resource kinds the UI cares about. */
export const WATCHED_KINDS: readonly WatchedKind[] = [
  {
    channel: 'pool',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'storagepools',
    namespaced: false,
  },
  {
    channel: 'dataset',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'datasets',
    namespaced: false,
  },
  {
    channel: 'bucket',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'buckets',
    namespaced: false,
  },
  {
    channel: 'share',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'shares',
    namespaced: false,
  },
  { channel: 'disk', group: 'novanas.io', version: 'v1alpha1', plural: 'disks', namespaced: false },
  {
    channel: 'snapshot',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'snapshots',
    namespaced: false,
  },
  {
    channel: 'app',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'appinstances',
    namespaced: true,
  },
  {
    channel: 'appcatalog',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'appcatalogs',
    namespaced: false,
  },
  {
    channel: 'vm',
    group: 'kubevirt.io',
    version: 'v1',
    plural: 'virtualmachines',
    namespaced: true,
  },
  {
    channel: 'isolib',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'isolibraries',
    namespaced: false,
  },
  {
    channel: 'objectstore',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'objectstores',
    namespaced: false,
  },
  {
    channel: 'bucketuser',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'bucketusers',
    namespaced: false,
  },
  {
    channel: 'smbserver',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'smbservers',
    namespaced: false,
  },
  {
    channel: 'nfsserver',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'nfsservers',
    namespaced: false,
  },
  {
    channel: 'iscsitarget',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'iscsitargets',
    namespaced: false,
  },
  {
    channel: 'nvmeoftarget',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'nvmeoftargets',
    namespaced: false,
  },
  { channel: 'user', group: 'novanas.io', version: 'v1alpha1', plural: 'users', namespaced: false },
  {
    channel: 'alert',
    group: 'novanas.io',
    version: 'v1alpha1',
    plural: 'alerts',
    namespaced: false,
  },
];

const CHANNEL_PREFIX = 'novanas:events:';
const RECONNECT_BASE_MS = 1000;
const RECONNECT_MAX_MS = 30_000;

interface K8sObject {
  metadata?: {
    name?: string;
    namespace?: string;
    resourceVersion?: string;
    uid?: string;
  };
}

export interface KubeWatchOptions {
  config: KubeConfig;
  redis: Redis;
  logger: FastifyBaseLogger;
  /** Override for tests. */
  kinds?: readonly WatchedKind[];
}

export interface KubeWatchHandle {
  stop(): Promise<void>;
}

export function startKubeWatch(opts: KubeWatchOptions): KubeWatchHandle {
  const kinds = opts.kinds ?? WATCHED_KINDS;
  const watch = new Watch(opts.config);
  const controllers: Array<{ abort: () => void; stopped: boolean }> = [];
  const lastResourceVersionByKind = new Map<string, string>();
  const seenResourceVersionByUid = new Map<string, string>();

  for (const kind of kinds) {
    const ctrl = { abort: () => {}, stopped: false };
    controllers.push(ctrl);
    let backoffMs = RECONNECT_BASE_MS;

    const connect = (): void => {
      if (ctrl.stopped) return;
      const path = kind.namespaced
        ? `/apis/${kind.group}/${kind.version}/${kind.plural}`
        : `/apis/${kind.group}/${kind.version}/${kind.plural}`;
      const queryParams: Record<string, string> = {};
      const rv = lastResourceVersionByKind.get(kind.channel);
      if (rv) queryParams.resourceVersion = rv;

      watch
        .watch(
          path,
          queryParams,
          (phase: string, apiObj: unknown) => {
            void handleEvent(kind, phase, apiObj as K8sObject);
            backoffMs = RECONNECT_BASE_MS; // healthy traffic resets backoff
          },
          (err: unknown) => {
            if (ctrl.stopped) return;
            opts.logger.warn({ err, kind: kind.channel }, 'kube_watch.stream_closed');
            const delay = Math.min(backoffMs, RECONNECT_MAX_MS);
            backoffMs = Math.min(backoffMs * 2, RECONNECT_MAX_MS);
            setTimeout(connect, delay).unref?.();
          }
        )
        .then((req: unknown) => {
          if (req && typeof (req as { abort?: () => void }).abort === 'function') {
            ctrl.abort = () => (req as { abort: () => void }).abort();
          }
        })
        .catch((err: unknown) => {
          opts.logger.error({ err, kind: kind.channel }, 'kube_watch.start_failed');
          const delay = Math.min(backoffMs, RECONNECT_MAX_MS);
          backoffMs = Math.min(backoffMs * 2, RECONNECT_MAX_MS);
          setTimeout(connect, delay).unref?.();
        });
    };

    const handleEvent = async (k: WatchedKind, phase: string, obj: K8sObject): Promise<void> => {
      const name = obj.metadata?.name;
      const namespace = obj.metadata?.namespace;
      const rv = obj.metadata?.resourceVersion;
      const uid = obj.metadata?.uid;
      if (!name || !rv) return;

      // Dedup by (uid, resourceVersion)
      if (uid) {
        const prev = seenResourceVersionByUid.get(uid);
        if (prev === rv) return;
        seenResourceVersionByUid.set(uid, rv);
      }
      lastResourceVersionByKind.set(k.channel, rv);

      const id = k.namespaced && namespace ? `${namespace}/${name}` : name;
      const channel = `${k.channel}:${id}`;
      const event = mapPhase(phase);
      const message = JSON.stringify({ event, payload: obj });
      try {
        await opts.redis.publish(`${CHANNEL_PREFIX}${channel}`, message);
      } catch (err) {
        opts.logger.warn({ err, channel }, 'kube_watch.publish_failed');
      }
    };

    connect();
  }

  return {
    async stop(): Promise<void> {
      for (const c of controllers) {
        c.stopped = true;
        try {
          c.abort();
        } catch {
          /* ignore */
        }
      }
    },
  };
}

function mapPhase(phase: string): string {
  switch (phase) {
    case 'ADDED':
      return 'added';
    case 'MODIFIED':
      return 'modified';
    case 'DELETED':
      return 'deleted';
    default:
      return phase.toLowerCase();
  }
}
