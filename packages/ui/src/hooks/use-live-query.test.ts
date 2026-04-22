import { renderHookWithClient } from '@/api/test-utils';
import { WsClient, __setWsClientForTests } from '@/lib/ws';
import { act, waitFor } from '@testing-library/react';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { useLiveQuery } from './use-live-query';

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
}

(globalThis as unknown as { WebSocket: typeof FakeWebSocket }).WebSocket =
  FakeWebSocket as unknown as typeof FakeWebSocket;

describe('useLiveQuery', () => {
  let client: WsClient;

  beforeEach(() => {
    FakeWebSocket.instances = [];
    client = new WsClient({
      url: 'ws://test/ws',
      wsFactory: (u) => new FakeWebSocket(u) as unknown as WebSocket,
      heartbeatMs: 60_000,
      pongTimeoutMs: 60_000,
    });
    __setWsClientForTests(client);
    FakeWebSocket.instances[0]?.open();
  });

  afterEach(() => {
    client.close();
    __setWsClientForTests(null);
  });

  it('refetches when a WS event arrives on the subscribed channel', async () => {
    const fn = vi.fn().mockResolvedValue({ n: 1 });
    const { result } = renderHookWithClient(() =>
      useLiveQuery(['pools'], fn, { wsChannel: 'pool:*' })
    );
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    expect(fn).toHaveBeenCalledTimes(1);

    // Dispatch an event on the channel; invalidation should trigger a refetch.
    await act(async () => {
      FakeWebSocket.instances[0]!.recv({
        channel: 'pool:*',
        event: 'updated',
        payload: { kind: 'StoragePool', name: 'fast' },
      });
    });
    await waitFor(() => expect(fn).toHaveBeenCalledTimes(2));
  });

  it('does not refetch when no wsChannel is provided', async () => {
    const fn = vi.fn().mockResolvedValue({ n: 1 });
    const { result } = renderHookWithClient(() => useLiveQuery(['pools'], fn));
    await waitFor(() => expect(result.current.isSuccess).toBe(true));
    await act(async () => {
      FakeWebSocket.instances[0]!.recv({
        channel: 'pool:*',
        event: 'updated',
        payload: {},
      });
    });
    // Only the initial fetch.
    expect(fn).toHaveBeenCalledTimes(1);
  });
});
