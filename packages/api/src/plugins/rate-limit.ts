import rateLimit from '@fastify/rate-limit';
import type { FastifyInstance } from 'fastify';

export async function registerRateLimit(app: FastifyInstance): Promise<void> {
  await app.register(rateLimit, {
    max: 300,
    timeWindow: '1 minute',
    // Don't rate-limit health/metrics probes
    allowList: (req) =>
      req.url === '/health' || req.url === '/livez' || req.url === '/readyz' || req.url === '/metrics',
  });
}
