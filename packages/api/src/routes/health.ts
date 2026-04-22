import type { FastifyInstance } from 'fastify';

export async function healthRoutes(app: FastifyInstance): Promise<void> {
  app.get(
    '/health',
    {
      schema: {
        description: 'Liveness + readiness combined probe.',
        tags: ['system'],
        response: {
          200: {
            type: 'object',
            properties: {
              status: { type: 'string' },
              uptime: { type: 'number' },
            },
            required: ['status', 'uptime'],
          },
        },
      },
    },
    async () => ({ status: 'ok', uptime: process.uptime() })
  );

  app.get(
    '/livez',
    { schema: { description: 'Kubernetes liveness probe.', tags: ['system'] } },
    async (_req, reply) => reply.code(200).send('ok')
  );

  app.get(
    '/readyz',
    { schema: { description: 'Kubernetes readiness probe.', tags: ['system'] } },
    async (_req, reply) => {
      // TODO(wave-3): verify Redis + DB connectivity before returning 200
      return reply.code(200).send('ready');
    }
  );
}
