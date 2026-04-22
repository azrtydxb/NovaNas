import { describe, expect, it, vi } from 'vitest';
import { type AuditEvent, writeAudit } from './audit.js';

function fakeLogger() {
  return {
    info: vi.fn(),
    warn: vi.fn(),
    error: vi.fn(),
    debug: vi.fn(),
    trace: vi.fn(),
    fatal: vi.fn(),
    level: 'info',
    silent: vi.fn(),
    child: vi.fn(),
  } as unknown as import('fastify').FastifyBaseLogger;
}

describe('writeAudit', () => {
  it('pino-logs even when db is null', async () => {
    const logger = fakeLogger();
    const event: AuditEvent = {
      actor: 'alice',
      action: 'dataset.create',
      kind: 'Dataset',
      outcome: 'success',
    };
    await writeAudit(null, logger, event);
    expect(logger.info).toHaveBeenCalledWith({ audit: event }, 'audit.event');
  });

  it('inserts a row into the auditLog table when db is provided', async () => {
    const logger = fakeLogger();
    const values = vi.fn().mockResolvedValue(undefined);
    const insert = vi.fn().mockReturnValue({ values });
    const db = { insert } as unknown as import('./db.js').DbClient;
    await writeAudit(db, logger, {
      actor: 'alice',
      actorId: '00000000-0000-0000-0000-000000000001',
      action: 'dataset.create',
      resourceKind: 'Dataset',
      resourceName: 'data-a',
      outcome: 'success',
      sourceIp: '127.0.0.1',
      payload: { spec: { size: '10Gi' } },
    });
    expect(insert).toHaveBeenCalledTimes(1);
    expect(values).toHaveBeenCalledTimes(1);
    const arg = values.mock.calls[0]![0] as Record<string, unknown>;
    expect(arg.action).toBe('dataset.create');
    expect(arg.resourceKind).toBe('Dataset');
    expect(arg.resourceName).toBe('data-a');
    expect(arg.outcome).toBe('success');
    expect(arg.sourceIp).toBe('127.0.0.1');
  });

  it('swallows db errors and logs them', async () => {
    const logger = fakeLogger();
    const db = {
      insert: () => ({
        values: () => Promise.reject(new Error('boom')),
      }),
    } as unknown as import('./db.js').DbClient;
    await expect(
      writeAudit(db, logger, {
        actor: 'x',
        action: 'x',
        outcome: 'failure',
      })
    ).resolves.toBeUndefined();
    expect(logger.error).toHaveBeenCalled();
  });
});
