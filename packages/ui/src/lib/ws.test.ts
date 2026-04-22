import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { WsClient, type WsStatus } from './ws';

/**
 * Fake WebSocket that records sent frames and lets tests drive open/message/close
 * transitions deterministically.
 */
class FakeWebSocket {
  static OPEN = 1;
  static CLOSED = 3;
  readyState = 0;
  listeners: Record<string, Array<(e: unknown) => void>> = {};
  sent: string[] = [];
  url: string;
  constructor(url: string) {
    this.url = url;
    FakeWebSocket.instances.push(this);
  }
  addEventListener(type: string, cb: (e: unknown) => void) {
    const list = this.listeners[type] ?? [];
    list.push(cb);
    this.listeners[type] = list;
  }
  send(data: string) {
    this.sent.push(data);
  }
  close() {
    this.readyState = FakeWebSocket.CLOSED;
    this.dispatch('close', {});
  }
  dispatch(type: string, ev: unknown) {
    for (const cb of this.listeners[type] ?? []) cb(ev);
  }
  open() {
    this.readyState = FakeWebSocket.OPEN;
    this.dispatch('open', {});
  }
  recv(obj: unknown) {
    this.dispatch('message', { data: JSON.stringify(obj) });
  }
  static instances: FakeWebSocket[] = [];
  static reset() {
    FakeWebSocket.instances = [];
  }
}

// Expose WebSocket static constants the client reads from the global.
(globalThis as unknown as { WebSocket: typeof FakeWebSocket }).WebSocket =
  FakeWebSocket as unknown as typeof FakeWebSocket;

function makeClient(overrides: Partial<ConstructorParameters<typeof WsClient>[0]> = {}) {
  return new WsClient({
    url: 'ws://test/ws',
    backoffMs: 10,
    maxBackoffMs: 40,
    heartbeatMs: 1_000,
    pongTimeoutMs: 2_000,
    wsFactory: (u) => new FakeWebSocket(u) as unknown as WebSocket,
    ...overrides,
  });
}

describe('WsClient', () => {
  beforeEach(() => {
    FakeWebSocket.reset();
    vi.useFakeTimers();
  });
  afterEach(() => {
    vi.useRealTimers();
  });

  it('connects and transitions status to open', () => {
    const client = makeClient();
    const statuses: WsStatus[] = [];
    client.onStatus((s) => statuses.push(s));
    const sock = FakeWebSocket.instances[0]!;
    sock.open();
    expect(client.status).toBe('open');
    expect(statuses).toContain('connecting');
    expect(statuses).toContain('open');
    client.close();
  });

  it('ref-counts subscriptions and sends one subscribe frame per channel', () => {
    const client = makeClient();
    const sock = FakeWebSocket.instances[0]!;
    sock.open();
    const h1 = vi.fn();
    const h2 = vi.fn();
    const unsub1 = client.subscribe('pool:*', h1);
    const unsub2 = client.subscribe('pool:*', h2);
    // Only one subscribe frame for two consumers.
    const subFrames = sock.sent.filter((s) => s.includes('"subscribe"'));
    expect(subFrames.length).toBe(1);

    // Both receive messages.
    sock.recv({ channel: 'pool:*', event: 'updated', payload: { a: 1 } });
    expect(h1).toHaveBeenCalledTimes(1);
    expect(h2).toHaveBeenCalledTimes(1);

    // Unsub one: no unsubscribe sent yet.
    unsub1();
    let unsubFrames = sock.sent.filter((s) => s.includes('"unsubscribe"'));
    expect(unsubFrames.length).toBe(0);

    // Second unsub triggers the wire unsubscribe.
    unsub2();
    unsubFrames = sock.sent.filter((s) => s.includes('"unsubscribe"'));
    expect(unsubFrames.length).toBe(1);

    client.close();
  });

  it('reconnects with backoff after unexpected close and re-subscribes', () => {
    const client = makeClient();
    const sock1 = FakeWebSocket.instances[0]!;
    sock1.open();
    const handler = vi.fn();
    client.subscribe('disk:*', handler);
    expect(sock1.sent.some((s) => s.includes('"subscribe"'))).toBe(true);

    // Simulate drop.
    sock1.close();
    expect(client.status).toBe('closed');

    // Advance past backoff.
    vi.advanceTimersByTime(20);
    expect(FakeWebSocket.instances.length).toBe(2);
    const sock2 = FakeWebSocket.instances[1]!;
    sock2.open();

    // New socket should have re-sent the subscribe.
    expect(sock2.sent.some((s) => s.includes('"subscribe"') && s.includes('disk:*'))).toBe(true);
    client.close();
  });

  it('handles pong and treats missing pong as disconnect', () => {
    const client = makeClient();
    const sock = FakeWebSocket.instances[0]!;
    sock.open();

    // Advance to heartbeat: client should send ping.
    vi.advanceTimersByTime(1_000);
    expect(sock.sent.some((s) => s.includes('"ping"'))).toBe(true);

    // No pong arrives — after pongTimeoutMs the socket is closed.
    vi.advanceTimersByTime(2_000);
    expect(sock.readyState).toBe(FakeWebSocket.CLOSED);
    client.close();
  });

  it('accepts both payload and legacy data field', () => {
    const client = makeClient();
    const sock = FakeWebSocket.instances[0]!;
    sock.open();
    const handler = vi.fn();
    client.subscribe('pool:*', handler);
    sock.recv({ channel: 'pool:*', event: 'u', data: { legacy: true } });
    sock.recv({ channel: 'pool:*', event: 'u', payload: { modern: true } });
    expect(handler).toHaveBeenNthCalledWith(1, { legacy: true }, 'u');
    expect(handler).toHaveBeenNthCalledWith(2, { modern: true }, 'u');
    client.close();
  });

  it('close() stops reconnection', () => {
    const client = makeClient();
    const sock = FakeWebSocket.instances[0]!;
    sock.open();
    client.close();
    vi.advanceTimersByTime(1_000);
    // No reconnect was scheduled after close.
    expect(FakeWebSocket.instances.length).toBe(1);
  });
});
