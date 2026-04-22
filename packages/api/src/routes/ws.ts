import { randomBytes } from 'node:crypto';
import type { FastifyInstance, FastifyRequest } from 'fastify';
import type { Env } from '../env.js';
import type { SessionStore } from '../auth/session.js';
import { userFromClaims } from '../auth/rbac.js';
import { isValidChannel } from '../ws/channels.js';
import type { WsHub } from '../ws/hub.js';

interface ClientFrame {
  op: 'subscribe' | 'unsubscribe' | 'ping';
  channel?: string;
}

export interface WsRouteDeps {
  env: Env;
  sessions: SessionStore;
  hub: WsHub;
}

export async function wsRoutes(app: FastifyInstance, deps: WsRouteDeps): Promise<void> {
  const { env, sessions, hub } = deps;

  app.get('/api/v1/ws', { websocket: true }, async (socket, req: FastifyRequest) => {
    // Authenticate via session cookie on the upgrade request
    const raw = req.cookies?.[env.SESSION_COOKIE_NAME];
    if (!raw) {
      socket.close(4401, 'unauthorized');
      return;
    }
    const unsigned = req.unsignCookie(raw);
    if (!unsigned.valid || !unsigned.value) {
      socket.close(4401, 'unauthorized');
      return;
    }
    const session = await sessions.touch(unsigned.value);
    if (!session) {
      socket.close(4401, 'unauthorized');
      return;
    }

    const user = userFromClaims(session.claims);
    const id = randomBytes(12).toString('base64url');
    hub.register({ id, socket, user, channels: new Set() });
    req.log.info({ wsId: id, user: user.username }, 'ws.client.connect');

    socket.on('message', (raw) => {
      let frame: ClientFrame;
      try {
        frame = JSON.parse(raw.toString()) as ClientFrame;
      } catch {
        socket.send(JSON.stringify({ error: 'invalid_json' }));
        return;
      }
      if (frame.op === 'ping') {
        socket.send(JSON.stringify({ op: 'pong', ts: Date.now() }));
        return;
      }
      if (!frame.channel || !isValidChannel(frame.channel)) {
        socket.send(JSON.stringify({ error: 'invalid_channel', channel: frame.channel }));
        return;
      }
      if (frame.op === 'subscribe') {
        hub.subscribe(id, frame.channel);
        socket.send(JSON.stringify({ ok: true, op: 'subscribed', channel: frame.channel }));
      } else if (frame.op === 'unsubscribe') {
        hub.unsubscribe(id, frame.channel);
        socket.send(JSON.stringify({ ok: true, op: 'unsubscribed', channel: frame.channel }));
      } else {
        socket.send(JSON.stringify({ error: 'invalid_op', op: frame.op }));
      }
    });

    socket.on('close', () => {
      hub.unregister(id);
      req.log.info({ wsId: id }, 'ws.client.disconnect');
    });
  });
}
