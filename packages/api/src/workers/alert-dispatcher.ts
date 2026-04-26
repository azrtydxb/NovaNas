// Alert dispatcher worker.
//
// Replaces the deleted operator-side AlertChannel reconciler's
// dispatch path (#51). Subscribes to `novanas:events:alerts` on the
// shared Redis pub/sub bus; for each incoming alert, looks up active
// AlertChannels in Postgres whose minSeverity admits the alert, and
// dispatches per-type:
//   - webhook: HTTP POST with the alert payload
//   - email / ntfy / pushover / slack / pagerduty: logged-only (the
//     SDK plumbing for those is tracked separately; this worker is
//     scaffolding so they all live in one place when we add them)
//
// Failures are logged and not retried in-process. The dispatcher is
// best-effort; durable retries belong to the planned jobs queue (also
// tracked under #51).

import type { AlertChannel } from '@novanas/schemas';
import type { Redis } from 'ioredis';
import type { Logger } from 'pino';
import type { PgResource } from '../services/pg-resource.js';

const CHANNEL_PREFIX = 'novanas:events:';
const ALERTS_CHANNEL = `${CHANNEL_PREFIX}alerts`;

export interface AlertEvent {
  /** Alert id (uuid or stable hash of the source). */
  id: string;
  severity: 'info' | 'warning' | 'critical';
  /** Short human-readable summary. */
  summary: string;
  /** Optional structured detail; passed through to webhook payloads. */
  detail?: Record<string, unknown>;
  /** Source-of-truth identifier (e.g. 'pool:tank/usage'). */
  source?: string;
  /** Epoch milliseconds when the alert fired. */
  firedAt: number;
}

export interface AlertDispatcherOptions {
  redisSub: Redis;
  /** PgResource for the AlertChannel kind. */
  channels: PgResource<AlertChannel>;
  logger: Logger;
  /** Override fetch (tests). */
  fetchImpl?: typeof fetch;
}

const SEVERITY_ORDER: Record<AlertEvent['severity'], number> = {
  info: 0,
  warning: 1,
  critical: 2,
};

function severityAtLeast(have: AlertEvent['severity'], min: AlertEvent['severity']): boolean {
  return SEVERITY_ORDER[have] >= SEVERITY_ORDER[min];
}

export class AlertDispatcher {
  private readonly fetchImpl: typeof fetch;
  private listening = false;

  constructor(private readonly opts: AlertDispatcherOptions) {
    this.fetchImpl = opts.fetchImpl ?? fetch;
  }

  async start(): Promise<void> {
    if (this.listening) return;
    this.listening = true;
    await this.opts.redisSub.subscribe(ALERTS_CHANNEL);
    this.opts.redisSub.on('message', (channel, raw) => {
      if (channel !== ALERTS_CHANNEL) return;
      let alert: AlertEvent;
      try {
        alert = JSON.parse(raw) as AlertEvent;
      } catch (err) {
        this.opts.logger.warn({ err }, 'alert-dispatcher.parse_failed');
        return;
      }
      // Fire-and-forget; don't block the subscriber loop.
      void this.dispatch(alert).catch((err) =>
        this.opts.logger.warn({ err, alertId: alert.id }, 'alert-dispatcher.dispatch_failed')
      );
    });
  }

  async stop(): Promise<void> {
    if (!this.listening) return;
    this.listening = false;
    await this.opts.redisSub.unsubscribe(ALERTS_CHANNEL).catch(() => undefined);
  }

  async dispatch(alert: AlertEvent): Promise<void> {
    const list = await this.opts.channels.list({});
    for (const ch of list.items) {
      const minSev = ch.spec.minSeverity ?? 'info';
      if (!severityAtLeast(alert.severity, minSev)) continue;
      try {
        await this.dispatchOne(ch, alert);
      } catch (err) {
        this.opts.logger.warn(
          { err, channel: ch.metadata.name, alertId: alert.id },
          'alert-dispatcher.channel_failed'
        );
      }
    }
  }

  private async dispatchOne(ch: AlertChannel, alert: AlertEvent): Promise<void> {
    switch (ch.spec.type) {
      case 'webhook': {
        if (!ch.spec.webhook?.url) return;
        const headers: Record<string, string> = {
          'content-type': 'application/json',
          ...(ch.spec.webhook.headers ?? {}),
        };
        const res = await this.fetchImpl(ch.spec.webhook.url, {
          method: 'POST',
          headers,
          body: JSON.stringify({
            id: alert.id,
            severity: alert.severity,
            summary: alert.summary,
            detail: alert.detail,
            source: alert.source,
            firedAt: alert.firedAt,
            channel: ch.metadata.name,
          }),
        });
        if (!res.ok) {
          throw new Error(
            `webhook ${ch.metadata.name} → ${res.status}: ${await res.text().catch(() => '')}`
          );
        }
        return;
      }
      case 'email':
      case 'ntfy':
      case 'pushover':
      case 'slack':
      case 'discord':
      case 'telegram':
      case 'browserPush':
        // SDK-backed dispatchers are tracked separately. Log so the
        // alert is at least visible until those land.
        this.opts.logger.info(
          { channel: ch.metadata.name, type: ch.spec.type, alertId: alert.id },
          'alert-dispatcher.unimplemented_type'
        );
        return;
      default:
        this.opts.logger.warn(
          { channel: ch.metadata.name, type: ch.spec.type },
          'alert-dispatcher.unknown_type'
        );
    }
  }
}
