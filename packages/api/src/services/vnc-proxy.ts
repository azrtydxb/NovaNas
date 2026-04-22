import type { FastifyBaseLogger } from 'fastify';
import WebSocket, { type RawData } from 'ws';

/**
 * VNC WebSocket proxy.
 *
 * Bridges a client WebSocket (browser, speaking RFB via noVNC) to KubeVirt's
 * `virtualmachineinstances/<name>/vnc` subresource. Proxies raw bytes
 * bidirectionally — the RFB handshake is performed end-to-end between the
 * browser's noVNC client and the KubeVirt-provided VNC server.
 *
 * On upstream disconnect, attempts a small number of exponential reconnects
 * within a short window; if still disconnected, closes the client socket.
 *
 * @remarks This module replaces the prior `spice-proxy`. spice-html5 is no
 * longer published on npm; we pivoted to VNC + noVNC (`@novnc/novnc`). The
 * byte-pumping mechanism is unchanged — only the upstream subresource and
 * naming differ.
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

export interface VncProxyOptions {
  vmNamespace: string;
  vmName: string;
  /** Console protocol. KubeVirt only exposes `vnc` — `spice` is accepted for
   * backward-compat and transparently falls through to the VNC subresource. */
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

export interface VncProxyHandle {
  cleanup(): void;
}

const OPEN_STATE = 1;
const DEFAULT_RECONNECT_WINDOW_MS = 5000;
const MAX_RECONNECT_ATTEMPTS = 4;

function buildUpstreamUrl(apiServerUrl: string, namespace: string, name: string): string {
  // KubeVirt subresource endpoint: `vnc` streams an RFB/VNC protocol over WS.
  const base = apiServerUrl.replace(/^http/i, 'ws').replace(/\/+$/, '');
  return `${base}/apis/subresources.kubevirt.io/v1/namespaces/${encodeURIComponent(
    namespace
  )}/virtualmachineinstances/${encodeURIComponent(name)}/vnc`;
}

export function createVncProxy(opts: VncProxyOptions): VncProxyHandle {
  const {
    vmNamespace,
    vmName,
    clientWs,
    apiServerUrl,
    apiServerToken,
    logger,
    upstreamFactory,
    reconnectWindowMs = DEFAULT_RECONNECT_WINDOW_MS,
  } = opts;

  const url = buildUpstreamUrl(apiServerUrl, vmNamespace, vmName);
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
      logger?.warn({ err, vmNamespace, vmName }, 'vnc.client.send_failed');
    }
  };

  const sendToUpstream = (data: unknown) => {
    if (disposed || !upstream) return;
    if (upstream.readyState !== undefined && upstream.readyState !== OPEN_STATE) return;
    try {
      upstream.send(data as never);
    } catch (err) {
      logger?.warn({ err, vmNamespace, vmName }, 'vnc.upstream.send_failed');
    }
  };

  const connect = () => {
    if (disposed) return;
    logger?.info({ vmNamespace, vmName, attempt: reconnectAttempts }, 'vnc.upstream.connecting');
    try {
      upstream = factory(url, headers);
    } catch (err) {
      logger?.error({ err, vmNamespace, vmName }, 'vnc.upstream.factory_failed');
      scheduleReconnect();
      return;
    }

    upstream.on('open', () => {
      logger?.info({ vmNamespace, vmName }, 'vnc.upstream.open');
      reconnectAttempts = 0;
    });
    upstream.on('message', (data: RawData) => {
      sendToClient(data);
    });
    upstream.on('close', (code: number, reason: Buffer) => {
      logger?.info(
        { vmNamespace, vmName, code, reason: reason?.toString?.() },
        'vnc.upstream.close'
      );
      if (!disposed) scheduleReconnect();
    });
    upstream.on('error', (err: Error) => {
      logger?.warn({ err, vmNamespace, vmName }, 'vnc.upstream.error');
    });
  };

  const scheduleReconnect = () => {
    if (disposed) return;
    const elapsed = Date.now() - startTime;
    if (elapsed >= reconnectWindowMs || reconnectAttempts >= MAX_RECONNECT_ATTEMPTS) {
      logger?.info({ vmNamespace, vmName }, 'vnc.upstream.giveup');
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
    logger?.warn({ err, vmNamespace, vmName }, 'vnc.client.error');
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
    logger?.info({ vmNamespace, vmName }, 'vnc.cleanup');
  };

  connect();

  return { cleanup };
}

/** @deprecated Use {@link createVncProxy}. Kept for backward compatibility. */
export const createSpiceProxy = createVncProxy;
/** @deprecated Use {@link VncProxyOptions}. */
export type SpiceProxyOptions = VncProxyOptions;
/** @deprecated Use {@link VncProxyHandle}. */
export type SpiceProxyHandle = VncProxyHandle;
