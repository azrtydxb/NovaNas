import type { Redis } from 'ioredis';
import type { FastifyBaseLogger } from 'fastify';
import type { WsHub } from './hub.js';

/**
 * Redis pub/sub fan-out. Operators and the API server publish domain
 * events to `novanas:events:<channel>`; the hub forwards to every
 * WebSocket subscriber. Single subscription per process.
 */
const CHANNEL_PREFIX = 'novanas:events:';

export class PubSub {
  constructor(
    private readonly sub: Redis,
    private readonly pub: Redis,
    private readonly hub: WsHub,
    private readonly logger: FastifyBaseLogger
  ) {}

  async start(): Promise<void> {
    await this.sub.psubscribe(`${CHANNEL_PREFIX}*`);
    this.sub.on('pmessage', (_pattern, redisChannel, message) => {
      if (!redisChannel.startsWith(CHANNEL_PREFIX)) return;
      const channel = redisChannel.slice(CHANNEL_PREFIX.length);
      try {
        const parsed = JSON.parse(message) as { event: string; payload: unknown };
        this.hub.broadcast(channel, parsed.event, parsed.payload);
      } catch (err) {
        this.logger.warn({ err, channel }, 'ws.pubsub.parse_failed');
      }
    });
  }

  async publish(channel: string, event: string, payload: unknown): Promise<void> {
    await this.pub.publish(
      `${CHANNEL_PREFIX}${channel}`,
      JSON.stringify({ event, payload })
    );
  }

  async stop(): Promise<void> {
    await this.sub.punsubscribe(`${CHANNEL_PREFIX}*`).catch(() => undefined);
  }
}
