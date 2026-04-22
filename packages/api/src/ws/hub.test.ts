import { describe, expect, it, vi } from 'vitest';
import type { AuthenticatedUser } from '../types.js';
import { WsHub } from './hub.js';

function fakeSocket() {
  return {
    readyState: 1,
    send: vi.fn(),
  } as unknown as import('ws').WebSocket;
}

const user: AuthenticatedUser = {
  sub: 'u',
  username: 'u',
  groups: [],
  roles: [],
  tenant: 'default',
  claims: {},
};

describe('WsHub', () => {
  it('broadcasts to subscribers on matching channel only', () => {
    const hub = new WsHub();
    const a = fakeSocket();
    const b = fakeSocket();
    hub.register({ id: 'a', socket: a, user, channels: new Set() });
    hub.register({ id: 'b', socket: b, user, channels: new Set() });
    hub.subscribe('a', 'pool:p1');
    const r = hub.broadcast('pool:p1', 'added', { name: 'p1' });
    expect(r.sent).toBe(1);
    expect(a.send).toHaveBeenCalled();
    expect(b.send).not.toHaveBeenCalled();
  });

  it('drops events over 10/sec per client', () => {
    const hub = new WsHub();
    const s = fakeSocket();
    hub.register({ id: 'a', socket: s, user, channels: new Set() });
    hub.subscribe('a', 'pool:x');
    let sent = 0;
    let dropped = 0;
    for (let i = 0; i < 15; i++) {
      const r = hub.broadcast('pool:x', 'modified', { i });
      sent += r.sent;
      dropped += r.dropped;
    }
    expect(sent).toBe(10);
    expect(dropped).toBe(5);
  });
});
