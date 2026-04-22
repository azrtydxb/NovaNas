import type { WebSocket } from 'ws';
import type { AuthenticatedUser } from '../types.js';

export interface WsClient {
  id: string;
  socket: WebSocket;
  user: AuthenticatedUser;
  channels: Set<string>;
  /** Rate-limit bookkeeping: sliding 1-second window of send timestamps. */
  sendWindow: number[];
  /** Number of events dropped due to rate limit since last warning. */
  dropped: number;
}

export interface BroadcastResult {
  sent: number;
  dropped: number;
}

const MAX_EVENTS_PER_SECOND = 10;

/**
 * In-memory WS client registry. Single-node NovaNas so no cross-pod
 * coordination is needed — the hub is just a map keyed by connection id.
 *
 * Enforces a soft rate limit of 10 events/sec per client; bursts beyond
 * the budget are dropped and counted in `client.dropped`.
 */
export class WsHub {
  private readonly clients = new Map<string, WsClient>();

  register(client: Omit<WsClient, 'sendWindow' | 'dropped'>): void {
    this.clients.set(client.id, { ...client, sendWindow: [], dropped: 0 });
  }

  unregister(id: string): void {
    this.clients.delete(id);
  }

  get(id: string): WsClient | undefined {
    return this.clients.get(id);
  }

  subscribe(id: string, channel: string): boolean {
    const c = this.clients.get(id);
    if (!c) return false;
    c.channels.add(channel);
    return true;
  }

  unsubscribe(id: string, channel: string): boolean {
    const c = this.clients.get(id);
    if (!c) return false;
    return c.channels.delete(channel);
  }

  /** Broadcast an event to every client subscribed to the channel. */
  broadcast(channel: string, event: string, payload: unknown): BroadcastResult {
    const frame = JSON.stringify({ channel, event, payload });
    const now = Date.now();
    let sent = 0;
    let dropped = 0;
    for (const c of this.clients.values()) {
      if (!c.channels.has(channel)) continue;
      if (c.socket.readyState !== 1 /* OPEN */) continue;
      // prune window
      const cutoff = now - 1000;
      while (c.sendWindow.length > 0 && c.sendWindow[0]! < cutoff) c.sendWindow.shift();
      if (c.sendWindow.length >= MAX_EVENTS_PER_SECOND) {
        c.dropped++;
        dropped++;
        continue;
      }
      c.sendWindow.push(now);
      c.socket.send(frame);
      sent++;
    }
    return { sent, dropped };
  }

  size(): number {
    return this.clients.size;
  }

  /** Snapshot of clients (for tests / introspection). */
  list(): WsClient[] {
    return Array.from(this.clients.values());
  }
}
