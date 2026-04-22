import { auditLog } from '@novanas/db';
import type { FastifyBaseLogger } from 'fastify';
import type { DbClient } from './db.js';

export type AuditActorType = 'user' | 'system' | 'operator' | 'apiToken';
export type AuditOutcome = 'success' | 'failure' | 'denied';

/**
 * AuditEvent — the shape callers use. Fields map to the `audit_log` table
 * in `@novanas/db`. Back-compat with pre-wave-10 callers: `actor`, `resource`,
 * `resourceId`, `kind`, `namespace`, `tenant`, `metadata` are still accepted.
 */
export interface AuditEvent {
  actor: string;
  /** Opaque user id for `actorId` FK. If omitted, only `actor` string is recorded in details. */
  actorId?: string | null;
  actorType?: AuditActorType;
  action: string;
  /** Resource kind (e.g. 'Dataset'). Preferred name; `kind` is legacy alias. */
  resourceKind?: string;
  resourceName?: string;
  resourceNamespace?: string | null;
  /** Legacy aliases kept for A9 callers. */
  resource?: string;
  resourceId?: string;
  kind?: string;
  namespace?: string;
  tenant?: string;
  outcome: AuditOutcome;
  /** Arbitrary JSON payload (request body, diff, etc.). */
  payload?: Record<string, unknown>;
  sourceIp?: string;
  ip?: string;
  userAgent?: string;
  details?: Record<string, unknown>;
  metadata?: Record<string, unknown>;
}

/**
 * Writes an AuditLog row.
 *
 * - Always logs a pino summary at info level for local debugging.
 * - Persists to Drizzle `auditLog` table when `db` is provided; silently
 *   skips the insert when `db` is null/undefined (tests).
 * - Never throws: DB failures are logged and swallowed so audit never
 *   blocks the request path.
 */
export async function writeAudit(
  db: DbClient | null | undefined,
  logger: FastifyBaseLogger,
  event: AuditEvent
): Promise<void> {
  logger.info({ audit: event }, 'audit.event');

  if (!db) return;

  const resourceKind = event.resourceKind ?? event.kind ?? event.resource ?? 'unknown';
  const resourceName = event.resourceName ?? event.resourceId;
  const resourceNamespace = event.resourceNamespace ?? event.namespace ?? null;
  const actorType: AuditActorType = event.actorType ?? 'user';
  const details: Record<string, unknown> = {
    ...(event.details ?? {}),
    ...(event.metadata ?? {}),
  };
  if (event.userAgent) details.userAgent = event.userAgent;
  if (event.tenant) details.tenant = event.tenant;
  if (event.actor && !event.actorId) details.actor = event.actor;

  try {
    await db.insert(auditLog).values({
      actorId: event.actorId ?? null,
      actorType,
      action: event.action,
      resourceKind,
      resourceName: resourceName ?? null,
      resourceNamespace,
      payload: event.payload ?? null,
      outcome: event.outcome,
      sourceIp: event.sourceIp ?? event.ip ?? null,
      details: Object.keys(details).length > 0 ? details : null,
    });
  } catch (err) {
    logger.error({ err, audit: event }, 'audit.persist_failed');
  }
}
