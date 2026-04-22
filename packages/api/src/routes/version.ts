import type { FastifyInstance } from 'fastify';
import type { Env } from '../env.js';

export async function versionRoutes(app: FastifyInstance, env: Env): Promise<void> {
  app.get(
    '/api/version',
    {
      schema: {
        description: 'API build + server version.',
        tags: ['system'],
        response: {
          200: {
            type: 'object',
            properties: {
              version: { type: 'string' },
              apiVersion: { type: 'string' },
              nodeVersion: { type: 'string' },
            },
            required: ['version', 'apiVersion', 'nodeVersion'],
          },
        },
      },
    },
    async () => ({
      version: env.API_VERSION,
      apiVersion: 'v1',
      nodeVersion: process.version,
    })
  );
}
