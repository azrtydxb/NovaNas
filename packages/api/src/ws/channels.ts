/**
 * WebSocket channel registry.
 *
 * Channels match a pattern against subscription requests. Each pattern
 * is a string like 'pool:*' where `*` is a single non-colon segment, or
 * a literal channel name. See docs/09-ui-and-api.md §WebSocket.
 */

export const CHANNEL_PATTERNS = [
  'events', // Kubernetes event stream
  'pool:*', // per-pool updates (pool name)
  'dataset:*', // per-dataset updates (id)
  'bucket:*', // per-bucket updates
  'share:*', // per-share updates
  'disk:*', // per-disk SMART / health
  'snapshot:*', // per-dataset snapshot timeline
  'app:*', // per-app lifecycle events
  'vm:*', // per-VM lifecycle
  'job:*', // long-running job progress (id)
  'alert', // alert stream
  'system', // system-wide status
] as const;

export type ChannelPattern = (typeof CHANNEL_PATTERNS)[number];

/**
 * Return true if `channel` matches any of the registered patterns.
 * Patterns only support trailing `*` wildcards on colon-separated segments.
 */
export function isValidChannel(channel: string): boolean {
  if (!channel || channel.length > 256) return false;
  for (const pat of CHANNEL_PATTERNS) {
    if (pat === channel) return true;
    if (pat.endsWith(':*')) {
      const prefix = pat.slice(0, -1); // e.g. 'pool:'
      if (channel.startsWith(prefix) && !channel.slice(prefix.length).includes(':')) {
        return true;
      }
    }
  }
  return false;
}

/** Parse a channel into (kind, id?) tuple for routing. */
export function parseChannel(channel: string): { kind: string; id?: string } {
  const idx = channel.indexOf(':');
  if (idx < 0) return { kind: channel };
  return { kind: channel.slice(0, idx), id: channel.slice(idx + 1) };
}
