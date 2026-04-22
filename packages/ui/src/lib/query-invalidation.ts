/**
 * Channel-to-query-key mapping.
 *
 * When a Kubernetes watcher publishes an event to a pub/sub channel and the
 * WS hub fans it out to the browser, we translate the channel name into the
 * set of TanStack Query keys that should be invalidated. This keeps the
 * mapping in one place so API hook modules can stay terse.
 *
 * Channel conventions (kube-watch.ts):
 *   <kind>:*              — any event for this resource kind
 *   <kind>:<name>         — events for a specific named resource
 *   <kind>:<ns>/<name>    — namespaced variants (apps, vms)
 */

export type QueryKey = readonly unknown[];

/**
 * Returns the list of query keys to invalidate in response to the given
 * channel. Unknown channels return an empty list.
 */
export function invalidationForChannel(channel: string): QueryKey[] {
  const [kindRaw, rest = ''] = channel.split(':', 2);
  const kind = kindRaw?.toLowerCase() ?? '';
  const name = rest === '*' ? '' : rest;

  switch (kind) {
    case 'pool': {
      const keys: QueryKey[] = [['pools']];
      if (name) keys.push(['pool', name]);
      return keys;
    }
    case 'dataset': {
      const keys: QueryKey[] = [['datasets']];
      if (name) keys.push(['dataset', name]);
      return keys;
    }
    case 'disk': {
      const keys: QueryKey[] = [['disks']];
      if (name) keys.push(['disk', name]);
      return keys;
    }
    case 'share': {
      const keys: QueryKey[] = [['shares']];
      if (name) keys.push(['share', name]);
      return keys;
    }
    case 'bucket': {
      const keys: QueryKey[] = [['buckets']];
      if (name) keys.push(['bucket', name]);
      return keys;
    }
    case 'snapshot': {
      const keys: QueryKey[] = [['snapshots']];
      if (name) keys.push(['snapshot', name]);
      return keys;
    }
    case 'app':
    case 'appinstance': {
      const keys: QueryKey[] = [['app-instances']];
      if (name) {
        const [ns, n] = name.includes('/') ? name.split('/', 2) : ['', name];
        if (n && ns) keys.push(['app-instance', ns, n]);
        if (n) keys.push(['app-instance', n]);
      }
      return keys;
    }
    case 'appsavailable':
    case 'app-available':
    case 'appcatalog':
    case 'appcatalogs': {
      return [['apps-available']];
    }
    case 'vm': {
      const keys: QueryKey[] = [['vms']];
      if (name) {
        const [ns, n] = name.includes('/') ? name.split('/', 2) : ['', name];
        if (n && ns) keys.push(['vm', ns, n]);
        if (n) keys.push(['vm', n]);
      }
      return keys;
    }
    case 'job': {
      const keys: QueryKey[] = [['jobs']];
      if (name) keys.push(['job', name]);
      return keys;
    }
    case 'alert': {
      const keys: QueryKey[] = [['alerts']];
      if (name) keys.push(['alerts', name]);
      return keys;
    }
    case 'system':
    case 'node': {
      return [['system']];
    }
    default:
      return [];
  }
}
