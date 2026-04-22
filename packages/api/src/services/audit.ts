import type { FastifyBaseLogger } from 'fastify';
import type { DbClient } from './db.js';

export interface AuditEvent {
  actor: string;
  action: string;
  resource?: string;
  resourceId?: string;
  tenant?: string;
  outcome: 'success' | 'failure';
  ip?: string;
  userAgent?: string;
  metadata?: Record<string, unknown>;
}

/**
 * Writes an AuditLog row. During scaffold we log to pino;
 * Wave 3 swaps this to `db.insert(auditLog).values(...)` once
 * `@novanas/db` exposes the typed table.
 */
export async function writeAudit(
  _db: DbClient,
  logger: FastifyBaseLogger,
  event: AuditEvent
): Promise<void> {
  logger.info({ audit: event }, 'audit.event');
  // TODO(wave-3): persist via Drizzle when @novanas/db ships auditLog table
}
