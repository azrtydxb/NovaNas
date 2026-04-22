import type { Redis } from 'ioredis';
import { describe, expect, it, vi } from 'vitest';
import type { AuthenticatedUser } from '../types.js';
import { WsHub } from './hub.js';
import { PubSub } from './pubsub.js';

/** Minimal in-process pub/sub that mimics ioredis's psubscribe+pmessage. */
function pairRedis(): { pub: Redis; sub: Redis } {
  type Handler = (pattern: string, channel: string, message: string) => void;
  const handlers: Handler[] = [];
  const sub = {
    psubscribe: vi.fn(async () => 1),
    punsubscribe: vi.fn(async () => 1),
    on(event: string, cb: Handler) {
      if (event === 'pmessage') handlers.push(cb);
      return sub;
    },
  } as unknown as Redis;
  const pub = {
    publish: vi.fn(async (channel: string, message: string) => {
      for (const h of handlers) h('novanas:events:*', channel, message);
      return 1;
    }),
  } as unknown as Redis;
  return { pub, sub };
}

function fakeLogger() {
  return {
    warn: vi.fn(),
    info: vi.fn(),
    error: vi.fn(),
  } as unknown as import('fastify').FastifyBaseLogger;
}

const user: AuthenticatedUser = {
  sub: 'u',
  username: 'u',
  groups: [],
  roles: [],
  tenant: 'default',
  claims: {},
};

describe('PubSub + Hub roundtrip', () => {
  it('publish is forwarded to subscribed clients via pattern handler', async () => {
    const hub = new WsHub();
    const { pub, sub } = pairRedis();
    const ps = new PubSub(sub, pub, hub, fakeLogger());
    await ps.start();

    const socket = { readyState: 1, send: vi.fn() } as unknown as import('ws').WebSocket;
    hub.register({ id: 'a', socket, user, channels: new Set() });
    hub.subscribe('a', 'pool:main');

    await ps.publish('pool:main', 'modified', { name: 'main' });

    expect(socket.send).toHaveBeenCalledTimes(1);
    const calls = (socket.send as unknown as { mock: { calls: string[][] } }).mock.calls;
    const frame = JSON.parse(calls[0]![0]!);
    expect(frame.channel).toBe('pool:main');
    expect(frame.event).toBe('modified');
    expect(frame.payload).toEqual({ name: 'main' });
  });
});
