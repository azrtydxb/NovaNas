import type { WebSocket } from 'ws';
import type { AuthenticatedUser } from '../types.js';

export interface WsClient {
  id: string;
  socket: WebSocket;
  user: AuthenticatedUser;
  channels: Set<string>;
}

/**
 * In-memory WS client registry. Single-node NovaNas so no cross-pod
 * coordination is needed — the hub is just a map keyed by connection id.
 */
export class WsHub {
  private readonly clients = new Map<string, WsClient>();

  register(client: WsClient): void {
    this.clients.set(client.id, client);
  }

  unregister(id: string): void {
    this.clients.delete(id);
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
  broadcast(channel: string, event: string, payload: unknown): number {
    const frame = JSON.stringify({ channel, event, payload });
    let sent = 0;
    for (const c of this.clients.values()) {
      if (!c.channels.has(channel)) continue;
      if (c.socket.readyState !== 1 /* OPEN */) continue;
      c.socket.send(frame);
      sent++;
    }
    return sent;
  }

  size(): number {
    return this.clients.size;
  }
}
