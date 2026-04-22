import swagger from '@fastify/swagger';
import swaggerUi from '@fastify/swagger-ui';
import type { FastifyInstance } from 'fastify';
import type { Env } from '../env.js';

export async function registerSwagger(app: FastifyInstance, env: Env): Promise<void> {
  await app.register(swagger, {
    openapi: {
      openapi: '3.1.0',
      info: {
        title: 'NovaNas API',
        description: 'Domain API for the NovaNas appliance.',
        version: env.API_VERSION,
      },
      servers: [{ url: env.API_PUBLIC_URL }],
      components: {
        securitySchemes: {
          sessionCookie: {
            type: 'apiKey',
            in: 'cookie',
            name: env.SESSION_COOKIE_NAME,
          },
        },
      },
      security: [{ sessionCookie: [] }],
    },
  });

  await app.register(swaggerUi, {
    routePrefix: '/docs',
    uiConfig: {
      docExpansion: 'list',
      deepLinking: false,
    },
  });
}
