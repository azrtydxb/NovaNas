import helmet from '@fastify/helmet';
import type { FastifyInstance } from 'fastify';

export async function registerHelmet(app: FastifyInstance): Promise<void> {
  await app.register(helmet, {
    // The web UI is served from a sibling origin (NovaNas UI package);
    // CSP is set by the UI layer. Keep Fastify helmet focused on API hardening.
    contentSecurityPolicy: false,
    crossOriginResourcePolicy: { policy: 'same-site' },
  });
}
