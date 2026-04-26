import type { DbClient } from '../services/db.js';
import type { FastifyInstance, FastifyReply } from 'fastify';
import { canAction } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import { buildCertificateResource, register as registerImpl } from '../resources/certificates.js';
import { accepted, nowIso, setAnnotationOnResource } from '../services/actions.js';
import type { AuthenticatedUser } from '../types.js';
import { registerUnavailable } from './_unavailable.js';

const GVR = { group: 'novanas.io', version: 'v1alpha1', plural: 'certificates' };

function forbid(reply: FastifyReply): FastifyReply {
  return reply.code(403).send({ error: 'forbidden', message: 'insufficient role' });
}

function registerCertificateActions(app: FastifyInstance, db: DbClient): void {
  const security = [{ sessionCookie: [] }];
  const resource = buildCertificateResource(db);

  // POST /api/v1/certificates/:name/renew
  app.route<{ Params: { name: string } }>({
    method: 'POST',
    url: '/api/v1/certificates/:name/renew',
    preHandler: requireAuth,
    schema: {
      summary: 'Trigger an early renewal of the certificate',
      tags: ['certificates'],
      security,
    },
    handler: async (req, reply) => {
      const user = req.user as AuthenticatedUser;
      if (!canAction(user, 'Certificate', 'renew')) return forbid(reply);
      try {
        await setAnnotationOnResource(
          resource,
          req.params.name,
          'novanas.io/action-renew',
          nowIso()
        );
        return accepted({ message: `renewal requested for ${req.params.name}` });
      } catch (err) {
        req.log.error({ err }, 'cert renew action failed');
        if ((err as { name?: string })?.name === 'PgNotFoundError') {
          return reply.code(404).send({ error: 'not_found', message: (err as Error).message });
        }
        return reply
          .code(500)
          .send({ error: 'internal_error', message: (err as Error)?.message ?? String(err) });
      }
    },
  });
}

export async function certificatesRoutes(
  app: FastifyInstance,
  db?: DbClient | null
): Promise<void> {
  if (db) {
    registerImpl(app, db);
    registerCertificateActions(app, db);
    return;
  }
  registerUnavailable(app, [
    {
      method: 'GET',
      url: '/api/v1/certificates',
      summary: 'List certificates',
      tag: 'certificates',
    },
    {
      method: 'POST',
      url: '/api/v1/certificates',
      summary: 'Create a certificate',
      tag: 'certificates',
    },
    {
      method: 'GET',
      url: '/api/v1/certificates/:name',
      summary: 'Get a certificate',
      tag: 'certificates',
    },
    {
      method: 'PATCH',
      url: '/api/v1/certificates/:name',
      summary: 'Update a certificate',
      tag: 'certificates',
    },
    {
      method: 'DELETE',
      url: '/api/v1/certificates/:name',
      summary: 'Delete a certificate',
      tag: 'certificates',
    },
  ]);
}
