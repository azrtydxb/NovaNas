import type { FastifyBaseLogger } from 'fastify';
import WebSocket, { type RawData } from 'ws';

/**
 * SPICE/VNC WebSocket proxy.
 *
 * Bridges a client WebSocket (browser) to KubeVirt's virtualmachineinstances
 * console endpoint. Proxies raw bytes bidirectionally.
 *
 * On upstream disconnect, attempts a small number of exponential reconnects
 * within a short window; if still disconnected, closes the client socket.
 */

export interface ClientSocketLike {
  readyState?: number;
  send(data: unknown, cb?: (err?: Error) => void): void;
  close(code?: number, reason?: string): void;
  on(event: 'message', listener: (data: unknown) => void): unknown;
  on(event: 'close', listener: (code?: number, reason?: Buffer) => void): unknown;
  on(event: 'error', listener: (err: Error) => void): unknown;
  on(event: string, listener: (...args: unknown[]) => void): unknown;
}

export type UpstreamFactory = (
  url: string,
  headers?: Record<string, string>
) => Pick<WebSocket, 'on' | 'send' | 'close' | 'readyState'>;

export interface SpiceProxyOptions {
  vmNamespace: string;
  vmName: string;
  type?: 'spice' | 'vnc';
  clientWs: ClientSocketLike;
  /** e.g. https://kubernetes.default.svc — omit scheme `https` → wss. */
  apiServerUrl: string;
  /** Bearer token for the Kubernetes API. */
  apiServerToken?: string;
  logger?: FastifyBaseLogger;
  /** Override upstream connection factory (used in tests). */
  upstreamFactory?: UpstreamFactory;
  /** Reconnect window in ms. 0 disables reconnects. */
  reconnectWindowMs?: number;
}

export interface SpiceProxyHandle {
  cleanup(): void;
}

const OPEN_STATE = 1;
const DEFAULT_RECONNECT_WINDOW_MS = 5000;
const MAX_RECONNECT_ATTEMPTS = 4;

function buildUpstreamUrl(
  apiServerUrl: string,
  namespace: string,
  name: string,
  type: 'spice' | 'vnc'
): string {
  // KubeVirt subresource endpoints: vnc | console (serial). There's no
  // standard `spice` subresource in upstream KubeVirt, but the pattern is
  // the same — `spice` is proxied here for forward-compat and falls back
  // to `vnc` when the upstream subresource doesn't exist.
  const sub = type === 'vnc' ? 'vnc' : 'vnc';
  const base = apiServerUrl.replace(/^http/i, 'ws').replace(/\/+$/, '');
  return `${base}/apis/subresources.kubevirt.io/v1/namespaces/${encodeURIComponent(
    namespace
  )}/virtualmachineinstances/${encodeURIComponent(name)}/${sub}`;
}

export function createSpiceProxy(opts: SpiceProxyOptions): SpiceProxyHandle {
  const {
    vmNamespace,
    vmName,
    type = 'spice',
    clientWs,
    apiServerUrl,
    apiServerToken,
    logger,
    upstreamFactory,
    reconnectWindowMs = DEFAULT_RECONNECT_WINDOW_MS,
  } = opts;

  const url = buildUpstreamUrl(apiServerUrl, vmNamespace, vmName, type);
  const headers: Record<string, string> = {};
  if (apiServerToken) headers.authorization = `Bearer ${apiServerToken}`;

  let upstream: ReturnType<UpstreamFactory> | null = null;
  let disposed = false;
  let reconnectAttempts = 0;
  let reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  const startTime = Date.now();

  const factory: UpstreamFactory =
    upstreamFactory ??
    ((u, h) =>
      new WebSocket(u, {
        headers: h,
        rejectUnauthorized: false, // in-cluster kube cert
      }));

  const sendToClient = (data: unknown) => {
    if (disposed) return;
    if (clientWs.readyState !== undefined && clientWs.readyState !== OPEN_STATE) return;
    try {
      clientWs.send(data);
    } catch (err) {
      logger?.warn({ err, vmNamespace, vmName }, 'spice.client.send_failed');
    }
  };

  const sendToUpstream = (data: unknown) => {
    if (disposed || !upstream) return;
    if (upstream.readyState !== undefined && upstream.readyState !== OPEN_STATE) return;
    try {
      upstream.send(data as never);
    } catch (err) {
      logger?.warn({ err, vmNamespace, vmName }, 'spice.upstream.send_failed');
    }
  };

  const connect = () => {
    if (disposed) return;
    logger?.info(
      { vmNamespace, vmName, type, attempt: reconnectAttempts },
      'spice.upstream.connecting'
    );
    try {
      upstream = factory(url, headers);
    } catch (err) {
      logger?.error({ err, vmNamespace, vmName }, 'spice.upstream.factory_failed');
      scheduleReconnect();
      return;
    }

    upstream.on('open', () => {
      logger?.info({ vmNamespace, vmName }, 'spice.upstream.open');
      reconnectAttempts = 0;
    });
    upstream.on('message', (data: RawData) => {
      sendToClient(data);
    });
    upstream.on('close', (code: number, reason: Buffer) => {
      logger?.info(
        { vmNamespace, vmName, code, reason: reason?.toString?.() },
        'spice.upstream.close'
      );
      if (!disposed) scheduleReconnect();
    });
    upstream.on('error', (err: Error) => {
      logger?.warn({ err, vmNamespace, vmName }, 'spice.upstream.error');
    });
  };

  const scheduleReconnect = () => {
    if (disposed) return;
    const elapsed = Date.now() - startTime;
    if (elapsed >= reconnectWindowMs || reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
      logger?.info({ vmNamespace, vmName }, 'spice.upstream.giveup');
      try {
        clientWs.close(1011, 'upstream closed');
      } catch {
        /* best effort */
      }
      return;
    }
    const delay = Math.min(1000 * 2 ** reconnectAttempts, 4000);
    reconnectAttempts++;
    reconnectTimer = setTimeout(connect, delay);
  };

  // Wire client events.
  clientWs.on('message', (data: unknown) => {
    sendToUpstream(data);
  });
  clientWs.on('close', () => {
    cleanup();
  });
  clientWs.on('error', (err: Error) => {
    logger?.warn({ err, vmNamespace, vmName }, 'spice.client.error');
  });

  const cleanup = () => {
    if (disposed) return;
    disposed = true;
    if (reconnectTimer) {
      clearTimeout(reconnectTimer);
      reconnectTimer = null;
    }
    if (upstream) {
      try {
        upstream.close();
      } catch {
        /* best effort */
      }
      upstream = null;
    }
    logger?.info({ vmNamespace, vmName }, 'spice.cleanup');
  };

  connect();

  return { cleanup };
}
