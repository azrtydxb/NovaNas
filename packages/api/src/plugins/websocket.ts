import websocket from '@fastify/websocket';
import type { FastifyInstance } from 'fastify';

export async function registerWebsocket(app: FastifyInstance): Promise<void> {
  await app.register(websocket, {
    options: {
      maxPayload: 1 * 1024 * 1024, // 1 MiB
      // Note: @fastify/websocket delegates upgrade to ws; auth is enforced
      // in the route handler via request.user / cookie check.
    },
  });
}
