import type { FastifyInstance } from 'fastify';
import { z } from 'zod';
import { requireAuth } from '../auth/decorators.js';
import type { PromClient, PromQueryRange, PromSeries } from '../services/prom.js';

export interface MetricsRouteDeps {
  prom: PromClient | null;
}

/** Parse a Prometheus-like range string ("1h", "7d", "30m") to seconds. */
export function parseRange(input: string | undefined, defaultSec: number): number {
  if (!input) return defaultSec;
  const m = /^(\d+)([smhd])$/.exec(input);
  if (!m) return defaultSec;
  const n = Number(m[1]);
  const unit = m[2];
  switch (unit) {
    case 's':
      return n;
    case 'm':
      return n * 60;
    case 'h':
      return n * 3600;
    case 'd':
      return n * 86400;
    default:
      return defaultSec;
  }
}

export function stepFor(rangeSec: number): number {
  // ~ 120 points per chart
  if (rangeSec <= 3600) return 30;
  if (rangeSec <= 6 * 3600) return 60;
  if (rangeSec <= 24 * 3600) return 300;
  if (rangeSec <= 7 * 86400) return 900;
  return 3600;
}

function buildRange(rangeStr: string | undefined, defaultSec: number): PromQueryRange {
  const total = parseRange(rangeStr, defaultSec);
  const end = new Date();
  const start = new Date(end.getTime() - total * 1000);
  return { start, end, stepSeconds: stepFor(total) };
}

interface MetricsResponse {
  scope: string;
  query: string;
  range: { start: string; end: string; stepSeconds: number };
  series: PromSeries[];
}

function pack(
  scope: string,
  query: string,
  range: PromQueryRange,
  series: PromSeries[]
): MetricsResponse {
  return {
    scope,
    query,
    range: {
      start: range.start.toISOString(),
      end: range.end.toISOString(),
      stepSeconds: range.stepSeconds,
    },
    series,
  };
}

const genericQuery = z.object({
  scope: z.string().max(128).default('adhoc'),
  query: z.string().min(1).max(4096),
  range: z.string().max(16).optional(),
});

export async function metricsRoutes(app: FastifyInstance, deps: MetricsRouteDeps): Promise<void> {
  const prom = deps.prom;

  function requireProm(reply: import('fastify').FastifyReply): PromClient | null {
    if (!prom) {
      reply.code(503).send({ error: 'prometheus_unavailable' });
      return null;
    }
    return prom;
  }

  app.get(
    '/api/v1/metrics/pool/:name/throughput',
    { preHandler: requireAuth, schema: { tags: ['metrics'] } },
    async (req, reply) => {
      const p = requireProm(reply);
      if (!p) return;
      const { name } = req.params as { name: string };
      const range = buildRange((req.query as { range?: string }).range, 3600);
      const query = `sum by (direction) (rate(novanas_pool_bytes_total{pool="${name}"}[1m]))`;
      const series = await p.query(query, range);
      return pack(`pool:${name}:throughput`, query, range, series);
    }
  );

  app.get(
    '/api/v1/metrics/pool/:name/capacity',
    { preHandler: requireAuth, schema: { tags: ['metrics'] } },
    async (req, reply) => {
      const p = requireProm(reply);
      if (!p) return;
      const { name } = req.params as { name: string };
      const range = buildRange((req.query as { range?: string }).range, 7 * 86400);
      const query = `novanas_pool_capacity_bytes{pool="${name}"}`;
      const series = await p.query(query, range);
      return pack(`pool:${name}:capacity`, query, range, series);
    }
  );

  app.get(
    '/api/v1/metrics/disk/:wwn/smart',
    { preHandler: requireAuth, schema: { tags: ['metrics'] } },
    async (req, reply) => {
      const p = requireProm(reply);
      if (!p) return;
      const { wwn } = req.params as { wwn: string };
      const range = buildRange((req.query as { range?: string }).range, 7 * 86400);
      const query = `smartmon_attribute_raw_value{wwn="${wwn}"}`;
      const series = await p.query(query, range);
      return pack(`disk:${wwn}:smart`, query, range, series);
    }
  );

  app.get(
    '/api/v1/metrics/app/:ns/:name/resources',
    { preHandler: requireAuth, schema: { tags: ['metrics'] } },
    async (req, reply) => {
      const p = requireProm(reply);
      if (!p) return;
      const { ns, name } = req.params as { ns: string; name: string };
      const range = buildRange((req.query as { range?: string }).range, 3600);
      const query = `sum by (container) (rate(container_cpu_usage_seconds_total{namespace="${ns}",pod=~"${name}.*"}[1m])) or sum by (container) (container_memory_working_set_bytes{namespace="${ns}",pod=~"${name}.*"})`;
      const series = await p.query(query, range);
      return pack(`app:${ns}/${name}:resources`, query, range, series);
    }
  );

  app.get(
    '/api/v1/metrics/vm/:ns/:name/cpu',
    { preHandler: requireAuth, schema: { tags: ['metrics'] } },
    async (req, reply) => {
      const p = requireProm(reply);
      if (!p) return;
      const { ns, name } = req.params as { ns: string; name: string };
      const range = buildRange((req.query as { range?: string }).range, 3600);
      const query = `rate(kubevirt_vmi_vcpu_seconds_total{namespace="${ns}",name="${name}"}[1m])`;
      const series = await p.query(query, range);
      return pack(`vm:${ns}/${name}:cpu`, query, range, series);
    }
  );

  app.get(
    '/api/v1/metrics',
    { preHandler: requireAuth, schema: { tags: ['metrics'] } },
    async (req, reply) => {
      const p = requireProm(reply);
      if (!p) return;
      const parsed = genericQuery.safeParse(req.query);
      if (!parsed.success) {
        return reply.code(400).send({ error: 'invalid_query', details: parsed.error.format() });
      }
      const range = buildRange(parsed.data.range, 3600);
      const series = await p.query(parsed.data.query, range);
      return pack(parsed.data.scope, parsed.data.query, range, series);
    }
  );
}
