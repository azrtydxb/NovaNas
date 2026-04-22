/**
 * Single-connection multiplexed WebSocket manager.
 *
 * Channels defined in docs/09-ui-and-api.md:
 *   pool:{name}, disk:{wwn}, job:{id}, metrics:{scope}, events,
 *   app:{ns}/{name}, vm:{ns}/{name}, console:{vm}
 *
 * Wire protocol (tentative; API team is authoritative):
 *   client → server:  { op: 'subscribe' | 'unsubscribe', channel: string }
 *   server → client:  { channel: string, event: string, data: unknown }
 */

type Listener = (data: unknown, event: string) => void;

export interface WsMessage {
  channel: string;
  event: string;
  data: unknown;
}

export interface WsClientOptions {
  url?: string;
  backoffMs?: number;
  maxBackoffMs?: number;
}

export class WsClient {
  private socket: WebSocket | null = null;
  private readonly listeners = new Map<string, Set<Listener>>();
  private readonly pendingSubscriptions = new Set<string>();
  private backoff: number;
  private readonly maxBackoff: number;
  private readonly url: string;
  private closed = false;

  constructor(opts: WsClientOptions = {}) {
    this.url =
      opts.url ??
      `${window.location.protocol === 'https:' ? 'wss' : 'ws'}://${window.location.host}/ws`;
    this.backoff = opts.backoffMs ?? 500;
    this.maxBackoff = opts.maxBackoffMs ?? 15_000;
    this.connect();
  }

  private connect(): void {
    if (this.closed) return;
    try {
      this.socket = new WebSocket(this.url);
    } catch {
      this.scheduleReconnect();
      return;
    }

    this.socket.addEventListener('open', () => {
      this.backoff = 500;
      for (const ch of this.pendingSubscriptions) this.sendRaw({ op: 'subscribe', channel: ch });
    });

    this.socket.addEventListener('message', (evt) => {
      try {
        const msg = JSON.parse(String(evt.data)) as WsMessage;
        const bucket = this.listeners.get(msg.channel);
        if (!bucket) return;
        for (const l of bucket) l(msg.data, msg.event);
      } catch {
        // ignore malformed frames
      }
    });

    this.socket.addEventListener('close', () => {
      this.socket = null;
      this.scheduleReconnect();
    });
    this.socket.addEventListener('error', () => {
      this.socket?.close();
    });
  }

  private scheduleReconnect(): void {
    if (this.closed) return;
    const delay = this.backoff;
    this.backoff = Math.min(this.backoff * 2, this.maxBackoff);
    setTimeout(() => this.connect(), delay);
  }

  private sendRaw(payload: unknown): void {
    if (this.socket?.readyState === WebSocket.OPEN) {
      this.socket.send(JSON.stringify(payload));
    }
  }

  subscribe(channel: string, listener: Listener): () => void {
    let bucket = this.listeners.get(channel);
    if (!bucket) {
      bucket = new Set();
      this.listeners.set(channel, bucket);
      this.pendingSubscriptions.add(channel);
      this.sendRaw({ op: 'subscribe', channel });
    }
    bucket.add(listener);
    return () => {
      bucket?.delete(listener);
      if (bucket && bucket.size === 0) {
        this.listeners.delete(channel);
        this.pendingSubscriptions.delete(channel);
        this.sendRaw({ op: 'unsubscribe', channel });
      }
    };
  }

  close(): void {
    this.closed = true;
    this.socket?.close();
  }
}

let singleton: WsClient | null = null;
export function getWsClient(): WsClient {
  if (!singleton) singleton = new WsClient();
  return singleton;
}
