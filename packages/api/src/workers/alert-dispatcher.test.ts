import type { AlertChannel } from '@novanas/schemas';
import type { Logger } from 'pino';
import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import type { PgResource } from '../services/pg-resource.js';
import { AlertDispatcher, type AlertEvent } from './alert-dispatcher.js';

function silentLogger(): Logger {
  return {
    info: vi.fn(),
    warn: vi.fn(),
    debug: vi.fn(),
    error: vi.fn(),
    child: () => silentLogger(),
  } as unknown as Logger;
}

function fakeChannels(items: AlertChannel[]): PgResource<AlertChannel> {
  return { list: vi.fn().mockResolvedValue({ items }) } as unknown as PgResource<AlertChannel>;
}

function alert(severity: AlertEvent['severity'] = 'warning'): AlertEvent {
  return {
    id: 'a1',
    severity,
    summary: 'disk usage high',
    detail: { used: 0.91 },
    source: 'pool:tank',
    firedAt: 1_700_000_000_000,
  };
}

function webhookChannel(name: string, url: string, minSeverity?: AlertEvent['severity']): AlertChannel {
  return {
    apiVersion: 'novanas.io/v1alpha1',
    kind: 'AlertChannel',
    metadata: { name },
    spec: {
      type: 'webhook',
      webhook: { url },
      minSeverity,
    },
  };
}

describe('alert-dispatcher', () => {
  let fetchImpl: ReturnType<typeof vi.fn>;

  beforeEach(() => {
    fetchImpl = vi.fn().mockResolvedValue(new Response(null, { status: 200 }));
  });
  afterEach(() => {
    vi.clearAllMocks();
  });

  it('dispatches webhook channels with the alert payload', async () => {
    const d = new AlertDispatcher({
      redisSub: { subscribe: vi.fn(), on: vi.fn(), unsubscribe: vi.fn() } as never,
      channels: fakeChannels([webhookChannel('ops', 'http://example.com/hook')]),
      logger: silentLogger(),
      fetchImpl,
    });
    await d.dispatch(alert());
    expect(fetchImpl).toHaveBeenCalledTimes(1);
    const [url, init] = fetchImpl.mock.calls[0]!;
    expect(url).toBe('http://example.com/hook');
    const body = JSON.parse((init as RequestInit).body as string);
    expect(body).toMatchObject({ id: 'a1', severity: 'warning', channel: 'ops' });
  });

  it('skips channels whose minSeverity exceeds the alert', async () => {
    const d = new AlertDispatcher({
      redisSub: { subscribe: vi.fn(), on: vi.fn(), unsubscribe: vi.fn() } as never,
      channels: fakeChannels([webhookChannel('crit', 'http://example.com', 'critical')]),
      logger: silentLogger(),
      fetchImpl,
    });
    await d.dispatch(alert('warning'));
    expect(fetchImpl).not.toHaveBeenCalled();
  });

  it('logs but does not throw on per-channel failure', async () => {
    fetchImpl.mockResolvedValueOnce(new Response('boom', { status: 502 }));
    const logger = silentLogger();
    const d = new AlertDispatcher({
      redisSub: { subscribe: vi.fn(), on: vi.fn(), unsubscribe: vi.fn() } as never,
      channels: fakeChannels([
        webhookChannel('a', 'http://broken.example.com'),
        webhookChannel('b', 'http://ok.example.com'),
      ]),
      logger,
      fetchImpl,
    });
    await d.dispatch(alert());
    // Both channels were attempted, even though the first failed.
    expect(fetchImpl).toHaveBeenCalledTimes(2);
    expect((logger.warn as ReturnType<typeof vi.fn>).mock.calls.length).toBeGreaterThan(0);
  });
});
