/**
 * Single-connection multiplexed WebSocket manager.
 *
 * Channels defined in docs/09-ui-and-api.md:
 *   pool:{name}, disk:{wwn}, job:{id}, metrics:{scope}, events,
 *   app:{ns}/{name}, vm:{ns}/{name}, console:{vm}
 *
 * Wire protocol (aligned with kube-watch plugin + WS hub):
 *   client → server:  { op: 'subscribe' | 'unsubscribe' | 'ping', channel?: string }
 *   server → client:  { channel: string, event: string, payload?: unknown, data?: unknown }
 *                     { op: 'pong' }
 *
 * Connection auth is cookie-based (session cookie travels with the upgrade
 * request automatically). A single shared socket per tab multiplexes all
 * channel subscriptions; subscriptions are ref-counted so multiple consumers
 * of the same channel only cost one server-side subscribe.
 */

type Listener = (data: unknown, event: string) => void;

export interface WsMessage {
  channel: string;
  event: string;
  /** New-style payload field (kube-watch). */
  payload?: unknown;
  /** Legacy data field — fall back if payload is absent. */
  data?: unknown;
}

export type WsStatus = 'connecting' | 'open' | 'closed' | 'error';

export type WsStatusListener = (status: WsStatus) => void;

export interface WsClientOptions {
  url?: string;
  /** Initial reconnect delay (ms). Doubles on each failure up to `maxBackoffMs`. */
  backoffMs?: number;
  /** Maximum reconnect delay (ms). Default 30s. */
  maxBackoffMs?: number;
  /** Heartbeat interval (ms). Default 20s. */
  heartbeatMs?: number;
  /** Pong timeout (ms). Default 40s. */
  pongTimeoutMs?: number;
  /** Factory for WebSocket instances — overridable in tests. */
  wsFactory?: (url: string) => WebSocket;
}

export class WsClient {
  private socket: WebSocket | null = null;
  private readonly listeners = new Map<string, Set<Listener>>();
  /** Ref-counted pending (=desired) subscriptions, persisted across reconnects. */
  private readonly subCounts = new Map<string, number>();
  private backoff: number;
  private readonly initialBackoff: number;
  private readonly maxBackoff: number;
  private readonly heartbeatMs: number;
  private readonly pongTimeoutMs: number;
  private readonly url: string;
  private readonly wsFactory: (url: string) => WebSocket;
  private closed = false;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private heartbeatTimer: ReturnType<typeof setInterval> | null = null;
  private pongTimer: ReturnType<typeof setTimeout> | null = null;
  private _status: WsStatus = 'closed';
  private readonly statusListeners = new Set<WsStatusListener>();

  constructor(opts: WsClientOptions = {}) {
    this.url =
      opts.url ??
      `${typeof window !== 'undefined' && window.location.protocol === 'https:' ? 'wss' : 'ws'}://${
        typeof window !== 'undefined' ? window.location.host : 'localhost'
      }/ws`;
    this.initialBackoff = opts.backoffMs ?? 1_000;
    this.backoff = this.initialBackoff;
    this.maxBackoff = opts.maxBackoffMs ?? 30_000;
    this.heartbeatMs = opts.heartbeatMs ?? 20_000;
    this.pongTimeoutMs = opts.pongTimeoutMs ?? 40_000;
    this.wsFactory = opts.wsFactory ?? ((url) => new WebSocket(url));
    this.connect();
  }

  get status(): WsStatus {
    return this._status;
  }

  onStatus(listener: WsStatusListener): () => void {
    this.statusListeners.add(listener);
    listener(this._status);
    return () => {
      this.statusListeners.delete(listener);
    };
  }

  private setStatus(next: WsStatus): void {
    if (this._status === next) return;
    this._status = next;
    for (const l of this.statusListeners) l(next);
  }

  private connect(): void {
    if (this.closed) return;
    this.setStatus('connecting');
    let sock: WebSocket;
    try {
      sock = this.wsFactory(this.url);
    } catch {
      this.setStatus('error');
      this.scheduleReconnect();
      return;
    }
    this.socket = sock;

    sock.addEventListener('open', () => {
      this.backoff = this.initialBackoff;
      this.setStatus('open');
      // Re-subscribe to every channel we care about.
      for (const ch of this.subCounts.keys()) {
        this.sendRaw({ op: 'subscribe', channel: ch });
      }
      this.startHeartbeat();
    });

    sock.addEventListener('message', (evt) => {
      this.resetPongTimer();
      let msg: WsMessage | { op?: string };
      try {
        msg = JSON.parse(String(evt.data));
      } catch {
        return;
      }
      if ((msg as { op?: string }).op === 'pong') return;
      const m = msg as WsMessage;
      if (!m.channel) return;
      const bucket = this.listeners.get(m.channel);
      if (!bucket) return;
      const data = m.payload !== undefined ? m.payload : m.data;
      for (const l of bucket) l(data, m.event);
    });

    sock.addEventListener('close', () => {
      this.socket = null;
      this.stopHeartbeat();
      this.setStatus('closed');
      this.scheduleReconnect();
    });

    sock.addEventListener('error', () => {
      this.setStatus('error');
      try {
        sock.close();
      } catch {
        // ignore
      }
    });
  }

  private scheduleReconnect(): void {
    if (this.closed) return;
    if (this.reconnectTimer) return;
    const delay = this.backoff;
    this.backoff = Math.min(this.backoff * 2, this.maxBackoff);
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }

  private startHeartbeat(): void {
    this.stopHeartbeat();
    this.heartbeatTimer = setInterval(() => {
      this.sendRaw({ op: 'ping' });
      if (!this.pongTimer) {
        this.pongTimer = setTimeout(() => {
          // No response within window: force reconnect.
          this.pongTimer = null;
          try {
            this.socket?.close();
          } catch {
            // ignore
          }
        }, this.pongTimeoutMs);
      }
    }, this.heartbeatMs);
  }

  private resetPongTimer(): void {
    if (this.pongTimer) {
      clearTimeout(this.pongTimer);
      this.pongTimer = null;
    }
  }

  private stopHeartbeat(): void {
    if (this.heartbeatTimer) {
      clearInterval(this.heartbeatTimer);
      this.heartbeatTimer = null;
    }
    this.resetPongTimer();
  }

  private sendRaw(payload: unknown): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      try {
        this.socket.send(JSON.stringify(payload));
      } catch {
        // ignore transient send failure — reconnect will resubscribe
      }
    }
  }

  subscribe(channel: string, listener: Listener): () => void {
    let bucket = this.listeners.get(channel);
    if (!bucket) {
      bucket = new Set();
      this.listeners.set(channel, bucket);
    }
    bucket.add(listener);

    const count = this.subCounts.get(channel) ?? 0;
    this.subCounts.set(channel, count + 1);
    if (count === 0) {
      this.sendRaw({ op: 'subscribe', channel });
    }

    return () => {
      const b = this.listeners.get(channel);
      b?.delete(listener);
      const cur = this.subCounts.get(channel) ?? 0;
      const next = cur - 1;
      if (next <= 0) {
        this.subCounts.delete(channel);
        this.listeners.delete(channel);
        this.sendRaw({ op: 'unsubscribe', channel });
      } else {
        this.subCounts.set(channel, next);
      }
    };
  }

  close(): void {
    this.closed = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.stopHeartbeat();
    try {
      this.socket?.close();
    } catch {
      // ignore
    }
    this.socket = null;
    this.setStatus('closed');
  }
}

let singleton: WsClient | null = null;
export function getWsClient(): WsClient {
  if (!singleton) {
    singleton = new WsClient();
    if (typeof window !== 'undefined') {
      window.addEventListener('beforeunload', () => {
        singleton?.close();
      });
    }
  }
  return singleton;
}

/** Test hook: replace or clear the shared singleton. */
export function __setWsClientForTests(client: WsClient | null): void {
  singleton = client;
}
