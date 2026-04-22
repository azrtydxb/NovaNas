import { useActiveAlerts } from '@/api/alerts';
import { useRecentAudit } from '@/api/audit';
import { useMetric } from '@/api/metrics';
import { usePools } from '@/api/pools';
import { useSystemHealth, useSystemInfo } from '@/api/system';
import { CapacityBar } from '@/components/common/capacity-bar';
import { PageHeader } from '@/components/common/page-header';
import { Stat } from '@/components/common/stat';
import { StatusDot } from '@/components/common/status-dot';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Card,
  CardActions,
  CardBody,
  CardDescription,
  CardHeader,
  CardTitle,
} from '@/components/ui/card';
import { Progress } from '@/components/ui/progress';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeaderCell,
  TableRow,
} from '@/components/ui/table';
import { formatBytes } from '@/lib/format';
import type { StatusTone } from '@/types';
import { createFileRoute } from '@tanstack/react-router';
import { Activity, BarChart3, RefreshCw, Shield } from 'lucide-react';

export const Route = createFileRoute('/_auth/dashboard')({
  component: DashboardPage,
});

function toneFromStatus(s: 'ok' | 'warn' | 'err' | undefined): StatusTone {
  if (s === 'warn') return 'warn';
  if (s === 'err') return 'err';
  return 'ok';
}

function DashboardPage() {
  const health = useSystemHealth();
  const info = useSystemInfo();
  const pools = usePools();
  const alerts = useActiveAlerts();
  const audit = useRecentAudit(20);

  // Throughput / IOPS series from the metrics API.
  const readMetric = useMetric('system', 'throughput_read', '1h');
  const writeMetric = useMetric('system', 'throughput_write', '1h');
  const iopsMetric = useMetric('system', 'iops', '1h');
  const netMetric = useMetric('system', 'network_throughput', '1h');

  const readSeries = extractSeries(readMetric.data?.series);
  const writeSeries = extractSeries(writeMetric.data?.series);
  const iopsSeries = extractSeries(iopsMetric.data?.series);
  const netSeries = extractSeries(netMetric.data?.series);

  const h = health.data;
  const tone = toneFromStatus(h?.status);
  const capacityPct =
    h?.capacity && h.capacity.totalBytes > 0
      ? Math.round((h.capacity.usedBytes / h.capacity.totalBytes) * 100)
      : 0;

  return (
    <>
      <PageHeader
        title='Dashboard'
        subtitle={
          info.data
            ? `${info.data.hostname} · NovaNas ${info.data.version} · ${info.data.timezone}`
            : 'Loading system info…'
        }
        actions={
          <>
            <Button
              variant='default'
              onClick={() => {
                health.refetch();
                pools.refetch();
                alerts.refetch();
                audit.refetch();
              }}
            >
              <RefreshCw size={13} /> Refresh
            </Button>
            <Button variant='default'>
              <BarChart3 size={13} /> Open Grafana
            </Button>
          </>
        }
      />

      {/* Health hero */}
      <div className='relative overflow-hidden grid grid-cols-[1fr_auto] gap-4 p-4 mb-3 bg-panel border border-border rounded-lg'>
        <div
          className='absolute inset-0 pointer-events-none'
          style={{
            background:
              'radial-gradient(600px 200px at 10% 0%, var(--accent-soft), transparent 60%)',
          }}
        />
        <div className='relative z-[1]'>
          <div
            className={`text-[10px] tracking-[0.12em] uppercase font-medium flex items-center gap-2 ${
              tone === 'ok' ? 'text-success' : tone === 'warn' ? 'text-warning' : 'text-danger'
            }`}
          >
            <StatusDot tone={tone} />
            {h?.status === 'err'
              ? 'Attention required'
              : h?.status === 'warn'
                ? 'Degraded'
                : 'Operational'}
          </div>
          <div className='text-[28px] font-semibold text-foreground tracking-tight mt-1.5 mb-1'>
            {h?.message ?? (health.isLoading ? 'Loading…' : 'All systems healthy')}
          </div>
          <div className='text-foreground-muted text-base max-w-[56ch]'>
            {h ? (
              <>
                {h.disks.active} disks active · {h.pools.online} pools online
                {h.lastScrubAt ? (
                  <>
                    {' '}
                    · Last scrub: <span className='mono text-foreground'>{h.lastScrubAt}</span>
                  </>
                ) : null}
                {h.lastConfigBackupAt ? (
                  <>
                    {' '}
                    · Last config backup:{' '}
                    <span className='mono text-foreground'>{h.lastConfigBackupAt}</span>
                  </>
                ) : null}
              </>
            ) : health.isError ? (
              <span className='text-danger'>Unable to reach /system/health.</span>
            ) : (
              <Skeleton className='h-4 w-64' />
            )}
          </div>
          <div className='grid grid-cols-3 gap-3 mt-4'>
            <div>
              <div className='text-2xs uppercase tracking-wider text-foreground-subtle'>
                Capacity
              </div>
              <div className='mono text-lg text-foreground tnum'>
                {h?.capacity ? formatBytes(h.capacity.usedBytes) : '—'}
                <span className='text-base text-foreground-subtle'>
                  {' '}
                  / {h?.capacity ? formatBytes(h.capacity.totalBytes) : '—'}
                </span>
              </div>
              <Progress value={capacityPct} className='mt-2 max-w-[160px]' />
            </div>
            <div>
              <div className='text-2xs uppercase tracking-wider text-foreground-subtle'>
                Apps running
              </div>
              <div className='mono text-lg text-foreground tnum'>
                {h?.apps?.running ?? '—'}
                <span className='text-base text-foreground-subtle'>
                  {' '}
                  / {h?.apps?.installed ?? '—'} installed
                </span>
              </div>
            </div>
            <div>
              <div className='text-2xs uppercase tracking-wider text-foreground-subtle'>
                VMs running
              </div>
              <div className='mono text-lg text-foreground tnum'>
                {h?.vms?.running ?? '—'}
                <span className='text-base text-foreground-subtle'>
                  {' '}
                  / {h?.vms?.defined ?? '—'} defined
                </span>
              </div>
            </div>
          </div>
        </div>
        <div className='relative z-[1] flex items-center justify-center'>
          <div className='w-[140px] h-[140px] rounded-full grid place-items-center border border-border-strong bg-surface/40'>
            <div className='flex flex-col items-center'>
              <div className='mono text-2xl text-foreground font-medium tnum'>{capacityPct}%</div>
              <div className='text-xs text-foreground-subtle'>
                of {h?.capacity ? formatBytes(h.capacity.totalBytes) : '—'} usable
              </div>
            </div>
          </div>
        </div>
      </div>

      {/* Metric cards */}
      <div className='grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3 mb-3'>
        <Stat
          label='Read'
          value={lastValue(readSeries).toFixed(0)}
          unit='MB/s'
          data={readSeries}
          color='var(--accent)'
        />
        <Stat
          label='Write'
          value={lastValue(writeSeries).toFixed(0)}
          unit='MB/s'
          data={writeSeries}
          color='oklch(0.72 0.14 320)'
        />
        <Stat
          label='IOPS'
          value={Math.round(lastValue(iopsSeries)).toLocaleString()}
          data={iopsSeries}
          color='oklch(0.78 0.14 160)'
        />
        <Stat
          label='Network'
          value={lastValue(netSeries).toFixed(1)}
          unit='Gb/s'
          data={netSeries}
          color='oklch(0.82 0.14 60)'
        />
      </div>

      {/* Pools + alerts */}
      <div className='grid grid-cols-12 gap-3 mb-3'>
        <Card className='col-span-12 xl:col-span-8'>
          <CardHeader>
            <CardTitle>Pools</CardTitle>
            <CardDescription>
              {pools.data ? `${pools.data.length} pool${pools.data.length === 1 ? '' : 's'}` : '…'}
            </CardDescription>
          </CardHeader>
          <CardBody flush>
            {pools.isLoading ? (
              <div className='p-3 flex flex-col gap-2'>
                <Skeleton className='h-6' />
                <Skeleton className='h-6' />
              </div>
            ) : pools.isError ? (
              <div className='p-3 text-sm text-danger'>
                Unable to load pools.{' '}
                <Button size='sm' variant='ghost' onClick={() => pools.refetch()}>
                  Retry
                </Button>
              </div>
            ) : (pools.data?.length ?? 0) === 0 ? (
              <div className='p-6 text-sm text-foreground-subtle'>No pools configured.</div>
            ) : (
              <Table>
                <TableHead>
                  <tr>
                    <TableHeaderCell>Name</TableHeaderCell>
                    <TableHeaderCell>Tier</TableHeaderCell>
                    <TableHeaderCell>Phase</TableHeaderCell>
                    <TableHeaderCell>Disks</TableHeaderCell>
                    <TableHeaderCell>Usage</TableHeaderCell>
                  </tr>
                </TableHead>
                <TableBody>
                  {pools.data!.map((p) => {
                    const phase = p.status?.phase ?? 'Pending';
                    const t =
                      phase === 'Active'
                        ? 'ok'
                        : phase === 'Degraded'
                          ? 'warn'
                          : phase === 'Failed'
                            ? 'err'
                            : 'idle';
                    const cap = p.status?.capacity;
                    return (
                      <TableRow key={p.metadata.name}>
                        <TableCell>
                          <StatusDot tone={t} className='mr-2' />
                          <span className='text-foreground font-medium'>{p.metadata.name}</span>
                        </TableCell>
                        <TableCell>
                          <Badge>{p.spec.tier}</Badge>
                        </TableCell>
                        <TableCell className='mono text-xs text-foreground-muted'>
                          {phase}
                        </TableCell>
                        <TableCell className='mono'>{p.status?.diskCount ?? 0}</TableCell>
                        <TableCell>
                          {cap ? (
                            <CapacityBar
                              used={cap.usedBytes}
                              total={cap.totalBytes}
                              label={formatBytes(cap.usedBytes)}
                            />
                          ) : (
                            <span className='text-foreground-subtle text-xs'>—</span>
                          )}
                        </TableCell>
                      </TableRow>
                    );
                  })}
                </TableBody>
              </Table>
            )}
          </CardBody>
        </Card>

        <Card className='col-span-12 xl:col-span-4'>
          <CardHeader>
            <CardTitle>Alerts & activity</CardTitle>
            <CardActions>
              <Activity size={14} className='text-foreground-subtle' />
            </CardActions>
          </CardHeader>
          <CardBody flush>
            {alerts.isLoading || audit.isLoading ? (
              <div className='p-3 flex flex-col gap-2'>
                <Skeleton className='h-5' />
                <Skeleton className='h-5' />
                <Skeleton className='h-5' />
              </div>
            ) : (
              <div className='flex flex-col'>
                {(alerts.data ?? []).slice(0, 3).map((a) => (
                  <div
                    key={a.id}
                    className='grid grid-cols-[14px_1fr_auto] items-start gap-2.5 px-3.5 py-2 border-b border-border text-sm'
                  >
                    <StatusDot
                      tone={a.severity === 'err' ? 'err' : a.severity === 'warn' ? 'warn' : 'info'}
                      className='mt-1'
                    />
                    <div className='text-foreground'>{a.title}</div>
                    <div className='mono text-xs text-foreground-subtle'>
                      {shortTime(a.createdAt)}
                    </div>
                  </div>
                ))}
                {(audit.data ?? []).slice(0, 6).map((ev) => (
                  <div
                    key={ev.id}
                    className='grid grid-cols-[14px_1fr_auto] items-start gap-2.5 px-3.5 py-2 border-b border-border last:border-0 text-sm'
                  >
                    <StatusDot tone={ev.tone ?? 'info'} className='mt-1' />
                    <div className='text-foreground'>{ev.message}</div>
                    <div className='mono text-xs text-foreground-subtle'>
                      {shortTime(ev.timestamp)}
                    </div>
                  </div>
                ))}
                {(alerts.data?.length ?? 0) === 0 && (audit.data?.length ?? 0) === 0 && (
                  <div className='p-4 text-sm text-foreground-subtle text-center'>
                    No recent activity.
                  </div>
                )}
              </div>
            )}
          </CardBody>
        </Card>
      </div>

      {/* Services */}
      <div className='grid grid-cols-12 gap-3'>
        <Card className='col-span-12 md:col-span-6 xl:col-span-8'>
          <CardHeader>
            <CardTitle>System services</CardTitle>
            <CardDescription>novanas-system</CardDescription>
          </CardHeader>
          <CardBody className='flex flex-col gap-2'>
            {h?.services && h.services.length > 0 ? (
              h.services.map((s) => (
                <div key={s.name} className='flex items-center justify-between py-0.5'>
                  <div className='flex items-center gap-2'>
                    <StatusDot tone={s.tone ?? 'ok'} />
                    <span className='mono text-sm'>{s.name}</span>
                  </div>
                  <span className='mono text-xs text-foreground-subtle'>{s.status}</span>
                </div>
              ))
            ) : (
              <div className='text-sm text-foreground-subtle'>
                {health.isLoading ? 'Loading services…' : 'No service data.'}
              </div>
            )}
          </CardBody>
        </Card>

        <Card className='col-span-12 md:col-span-6 xl:col-span-4'>
          <CardHeader>
            <CardTitle>Protection</CardTitle>
            <CardActions>
              <Shield size={14} className='text-foreground-subtle' />
            </CardActions>
          </CardHeader>
          <CardBody className='flex flex-col gap-3 text-sm'>
            <div className='flex items-center justify-between'>
              <span className='text-foreground-muted'>Active alerts</span>
              <span className='mono text-foreground'>{alerts.data?.length ?? 0}</span>
            </div>
            <div className='flex items-center justify-between'>
              <span className='text-foreground-muted'>Last scrub</span>
              <span className='mono text-foreground'>{h?.lastScrubAt ?? '—'}</span>
            </div>
            <div className='flex items-center justify-between'>
              <span className='text-foreground-muted'>Last config backup</span>
              <span className='mono text-foreground'>{h?.lastConfigBackupAt ?? '—'}</span>
            </div>
          </CardBody>
        </Card>
      </div>
    </>
  );
}

function extractSeries(
  series: Array<{ points: Array<{ t: number; v: number }> }> | undefined
): number[] {
  if (!series || series.length === 0) return [];
  return series[0]!.points.map((p) => p.v);
}

function lastValue(series: number[]): number {
  if (series.length === 0) return 0;
  return series[series.length - 1] ?? 0;
}

function shortTime(iso: string): string {
  try {
    return new Date(iso).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
  } catch {
    return iso;
  }
}
