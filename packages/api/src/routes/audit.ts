import { auditLog } from '@novanas/db';
import { and, desc, eq, gte, lt, lte } from 'drizzle-orm';
import type { FastifyInstance } from 'fastify';
import { z } from 'zod';
import { AuthzRole } from '../auth/authz.js';
import { requireAuth } from '../auth/decorators.js';
import type { DbClient } from '../services/db.js';

export interface AuditRouteDeps {
  db: DbClient | null;
}

const listQuery = z.object({
  limit: z.coerce.number().int().positive().max(500).default(50),
  cursor: z.string().datetime().optional(),
  actor: z.string().uuid().optional(),
  kind: z.string().max(128).optional(),
  from: z.string().datetime().optional(),
  to: z.string().datetime().optional(),
  outcome: z.enum(['success', 'failure', 'denied']).optional(),
});

function isAdmin(roles: string[]): boolean {
  return roles.includes(AuthzRole.Admin);
}

export async function auditRoutes(app: FastifyInstance, deps: AuditRouteDeps): Promise<void> {
  const db = deps.db;

  app.get(
    '/api/v1/audit',
    {
      preHandler: requireAuth,
      schema: { tags: ['audit'], summary: 'List audit log entries' },
    },
    async (req, reply) => {
      if (!db) return reply.code(503).send({ error: 'db_unavailable' });
      const parsed = listQuery.safeParse(req.query);
      if (!parsed.success) {
        return reply.code(400).send({ error: 'invalid_query', details: parsed.error.format() });
      }
      const q = parsed.data;
      const user = req.user!;
      const admin = isAdmin(user.roles);

      const conditions = [];
      // Admins may filter by actor; non-admins are locked to their own sub.
      if (!admin) {
        conditions.push(eq(auditLog.actorId, user.sub));
      } else if (q.actor) {
        conditions.push(eq(auditLog.actorId, q.actor));
      }
      if (q.kind) conditions.push(eq(auditLog.resourceKind, q.kind));
      if (q.outcome) conditions.push(eq(auditLog.outcome, q.outcome));
      if (q.from) conditions.push(gte(auditLog.timestamp, new Date(q.from)));
      if (q.to) conditions.push(lte(auditLog.timestamp, new Date(q.to)));
      if (q.cursor) conditions.push(lt(auditLog.timestamp, new Date(q.cursor)));

      const where = conditions.length > 0 ? and(...conditions) : undefined;
      const base = db.select().from(auditLog);
      const filtered = where ? base.where(where) : base;
      const items = await filtered.orderBy(desc(auditLog.timestamp)).limit(q.limit);

      const nextCursor =
        items.length === q.limit ? items[items.length - 1]!.timestamp.toISOString() : null;

      return { items, nextCursor };
    }
  );
}
