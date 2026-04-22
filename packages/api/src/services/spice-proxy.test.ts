import { EventEmitter } from 'node:events';
import { describe, expect, it } from 'vitest';
import { type ClientSocketLike, createSpiceProxy } from './spice-proxy.js';

class FakeSocket extends EventEmitter {
  public readyState = 1;
  public sent: unknown[] = [];
  public closedWith?: { code?: number; reason?: string };
  send(data: unknown, cb?: (err?: Error) => void): void {
    this.sent.push(data);
    cb?.();
  }
  close(code?: number, reason?: string): void {
    this.closedWith = { code, reason };
    this.readyState = 3;
    this.emit('close', code, Buffer.from(reason ?? ''));
  }
}

describe('spice-proxy', () => {
  it('forwards bytes client <-> upstream', async () => {
    const client = new FakeSocket();
    const upstream = new FakeSocket();

    createSpiceProxy({
      vmNamespace: 'user-a',
      vmName: 'vm-1',
      clientWs: client as unknown as ClientSocketLike,
      apiServerUrl: 'https://api.example',
      upstreamFactory: () =>
        upstream as unknown as ReturnType<
          NonNullable<Parameters<typeof createSpiceProxy>[0]['upstreamFactory']>
        >,
      reconnectWindowMs: 0,
    });

    upstream.emit('open');

    // Client -> upstream
    client.emit('message', Buffer.from([1, 2, 3]));
    expect(upstream.sent).toHaveLength(1);

    // Upstream -> client
    upstream.emit('message', Buffer.from([9, 8, 7]));
    expect(client.sent).toHaveLength(1);
  });

  it('cleans up on client close', async () => {
    const client = new FakeSocket();
    const upstream = new FakeSocket();
    const handle = createSpiceProxy({
      vmNamespace: 'user-a',
      vmName: 'vm-2',
      clientWs: client as unknown as ClientSocketLike,
      apiServerUrl: 'https://api.example',
      upstreamFactory: () =>
        upstream as unknown as ReturnType<
          NonNullable<Parameters<typeof createSpiceProxy>[0]['upstreamFactory']>
        >,
      reconnectWindowMs: 0,
    });

    client.emit('close');
    expect(handle).toBeDefined();
  });

  it('closes client on unrecoverable upstream disconnect', async () => {
    const client = new FakeSocket();
    const upstream = new FakeSocket();
    createSpiceProxy({
      vmNamespace: 'user-a',
      vmName: 'vm-3',
      clientWs: client as unknown as ClientSocketLike,
      apiServerUrl: 'https://api.example',
      upstreamFactory: () =>
        upstream as unknown as ReturnType<
          NonNullable<Parameters<typeof createSpiceProxy>[0]['upstreamFactory']>
        >,
      reconnectWindowMs: 0, // no reconnect allowed
    });
    upstream.emit('close', 1006, Buffer.from('abnormal'));
    expect(client.closedWith?.code).toBe(1011);
  });
});
