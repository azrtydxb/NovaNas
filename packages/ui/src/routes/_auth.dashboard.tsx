import { createFileRoute } from '@tanstack/react-router';
import { Activity, BarChart3, Plus, RefreshCw, Shield } from 'lucide-react';
import { useEffect, useState } from 'react';
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
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeaderCell,
  TableRow,
} from '@/components/ui/table';
import { formatBytes } from '@/lib/format';

export const Route = createFileRoute('/_auth/dashboard')({
  component: DashboardPage,
});

// -----------------------------------------------------------------------------
// Mocked data — replaced by API-backed queries once /api/v1/dashboard is live.
// -----------------------------------------------------------------------------

const POOLS = [
  { name: 'fast', tier: 'hot', protection: 'rep×2', disks: 8, device: 'NVMe', used: 4.9e12, total: 7.6e12, r: 1420, w: 680 },
  { name: 'bulk', tier: 'warm', protection: 'EC 6+2', disks: 10, device: 'HDD', used: 23.1e12, total: 60e12, r: 340, w: 210 },
  { name: 'archive', tier: 'cold', protection: 'EC 4+2', disks: 6, device: 'HDD', used: 14e12, total: 30e12, r: 120, w: 80 },
  { name: 'meta', tier: 'hot', protection: 'rep×3', disks: 3, device: 'NVMe', used: 12e9, total: 500e9, r: 60, w: 30 },
];

const ACTIVITY: Array<{ tone: 'ok' | 'warn' | 'err' | 'info'; text: React.ReactNode; t: string }> = [
  { tone: 'ok', text: <>Snapshot <b>family-media@auto-14:58</b> taken (1.4 GB).</>, t: '14:58' },
  { tone: 'info', text: <>Replication <b>photos → offsite</b> started.</>, t: '14:47' },
  { tone: 'warn', text: <>Disk in slot 13 reports 5 reallocated sectors.</>, t: '13:12' },
  { tone: 'ok', text: <>Scrub on pool <b>bulk</b> progressing (18%).</>, t: '12:40' },
  { tone: 'ok', text: <>Config backup <b>14:00</b> uploaded to S3.</>, t: '14:00' },
];

// Build a deterministic sparkline series once at mount and nudge it occasionally.
function useSparkSeries(length: number, baseline: number, volatility: number, seed: number) {
  const [series, setSeries] = useState<number[]>(() =>
    Array.from({ length }, (_, i) => {
      const v = Math.sin((seed * 9301 + i * 49297) % 233280) * 43758.5453;
      return baseline + ((v - Math.floor(v)) - 0.5) * volatility * 2;
    })
  );
  useEffect(() => {
    const id = setInterval(() => {
      setSeries((prev) => {
        const next = prev.slice(1);
        const last = prev[prev.length - 1] ?? baseline;
        const target = baseline + (Math.random() - 0.5) * volatility * 4;
        next.push(Math.max(0, Math.min(1, last * 0.6 + target * 0.4)));
        return next;
      });
    }, 1500);
    return () => clearInterval(id);
  }, [baseline, volatility]);
  return series;
}

function DashboardPage() {
  const readSeries = useSparkSeries(48, 0.55, 0.18, 11);
  const writeSeries = useSparkSeries(48, 0.3, 0.15, 12);
  const iopsSeries = useSparkSeries(48, 0.48, 0.22, 13);
  const netSeries = useSparkSeries(48, 0.4, 0.2, 14);

  const readNow = Math.round((readSeries[readSeries.length - 1] ?? 0) * 2400);
  const writeNow = Math.round((writeSeries[writeSeries.length - 1] ?? 0) * 1200);
  const iopsNow = Math.round((iopsSeries[iopsSeries.length - 1] ?? 0) * 52000);
  const netNow = ((netSeries[netSeries.length - 1] ?? 0) * 8.5).toFixed(1);

  return (
    <>
      <PageHeader
        title='Dashboard'
        subtitle='nas-01 · NovaNas 26.07.3 · Europe/Brussels'
        actions={
          <>
            <Button variant='default'>
              <RefreshCw size={13} /> Refresh
            </Button>
            <Button variant='default'>
              <BarChart3 size={13} /> Open Grafana
            </Button>
            <Button variant='primary'>
              <Plus size={13} /> Create
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
          <div className='text-[10px] tracking-[0.12em] uppercase text-success font-medium flex items-center gap-2'>
            <StatusDot tone='ok' />
            Operational
          </div>
          <div className='text-[28px] font-semibold text-foreground tracking-tight mt-1.5 mb-1'>
            All systems healthy
          </div>
          <div className='text-foreground-muted text-base max-w-[56ch]'>
            22 disks active · 4 pools online · Keycloak, OpenBao, k3s control plane nominal. Last
            scrub: <span className='mono text-foreground'>3h ago</span>. Last config backup:{' '}
            <span className='mono text-foreground'>14:00</span>.
          </div>
          <div className='grid grid-cols-3 gap-3 mt-4'>
            <div>
              <div className='text-2xs uppercase tracking-wider text-foreground-subtle'>
                Capacity
              </div>
              <div className='mono text-lg text-foreground tnum'>
                42.1
                <span className='text-base text-foreground-subtle'> / 98.1 TB</span>
              </div>
              <Progress value={43} className='mt-2 max-w-[160px]' />
            </div>
            <div>
              <div className='text-2xs uppercase tracking-wider text-foreground-subtle'>
                Apps running
              </div>
              <div className='mono text-lg text-foreground tnum'>
                8<span className='text-base text-foreground-subtle'> / 12 installed</span>
              </div>
            </div>
            <div>
              <div className='text-2xs uppercase tracking-wider text-foreground-subtle'>
                VMs running
              </div>
              <div className='mono text-lg text-foreground tnum'>
                5<span className='text-base text-foreground-subtle'> / 6 defined</span>
              </div>
            </div>
          </div>
        </div>
        <div className='relative z-[1] flex items-center justify-center'>
          <div className='w-[140px] h-[140px] rounded-full grid place-items-center border border-border-strong bg-surface/40'>
            <div className='flex flex-col items-center'>
              <div className='mono text-2xl text-foreground font-medium tnum'>43%</div>
              <div className='text-xs text-foreground-subtle'>of 98.1 TB usable</div>
            </div>
          </div>
        </div>
      </div>

      {/* Metric cards */}
      <div className='grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-4 gap-3 mb-3'>
        <Stat label='Read' value={readNow} unit='MB/s' delta='+4.2%' up data={readSeries} color='var(--accent)' />
        <Stat label='Write' value={writeNow} unit='MB/s' delta='-1.1%' down data={writeSeries} color='oklch(0.72 0.14 320)' />
        <Stat label='IOPS' value={iopsNow.toLocaleString()} delta='+7.8%' up data={iopsSeries} color='oklch(0.78 0.14 160)' />
        <Stat label='Network' value={netNow} unit='Gb/s' delta='+0.4%' up data={netSeries} color='oklch(0.82 0.14 60)' />
      </div>

      {/* Pools + activity */}
      <div className='grid grid-cols-12 gap-3 mb-3'>
        <Card className='col-span-12 xl:col-span-8'>
          <CardHeader>
            <CardTitle>Pools</CardTitle>
            <CardDescription>4 online</CardDescription>
          </CardHeader>
          <CardBody flush>
            <Table>
              <TableHead>
                <tr>
                  <TableHeaderCell>Name</TableHeaderCell>
                  <TableHeaderCell>Tier</TableHeaderCell>
                  <TableHeaderCell>Protection</TableHeaderCell>
                  <TableHeaderCell>Disks</TableHeaderCell>
                  <TableHeaderCell>Usage</TableHeaderCell>
                  <TableHeaderCell className='text-right'>Read</TableHeaderCell>
                  <TableHeaderCell className='text-right'>Write</TableHeaderCell>
                </tr>
              </TableHead>
              <TableBody>
                {POOLS.map((p) => (
                  <TableRow key={p.name}>
                    <TableCell>
                      <StatusDot tone='ok' className='mr-2' />
                      <span className='text-foreground font-medium'>{p.name}</span>
                    </TableCell>
                    <TableCell>
                      <Badge>{p.tier}</Badge>
                    </TableCell>
                    <TableCell>
                      <Badge>{p.protection}</Badge>
                    </TableCell>
                    <TableCell className='mono'>
                      {p.disks}× {p.device}
                    </TableCell>
                    <TableCell>
                      <CapacityBar used={p.used} total={p.total} label={formatBytes(p.used)} />
                    </TableCell>
                    <TableCell className='mono text-right tnum'>{p.r} MB/s</TableCell>
                    <TableCell className='mono text-right tnum'>{p.w} MB/s</TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          </CardBody>
        </Card>

        <Card className='col-span-12 xl:col-span-4'>
          <CardHeader>
            <CardTitle>Alerts & activity</CardTitle>
            <CardActions>
              <Button variant='ghost' size='sm'>
                View all
              </Button>
            </CardActions>
          </CardHeader>
          <CardBody flush>
            <div className='flex flex-col'>
              {ACTIVITY.map((a, i) => (
                <div
                  key={i}
                  className='grid grid-cols-[14px_1fr_auto] items-start gap-2.5 px-3.5 py-2 border-b border-border last:border-0 text-sm'
                >
                  <StatusDot tone={a.tone} className='mt-1' />
                  <div className='text-foreground'>{a.text}</div>
                  <div className='mono text-xs text-foreground-subtle'>{a.t}</div>
                </div>
              ))}
            </div>
          </CardBody>
        </Card>
      </div>

      {/* Jobs + services */}
      <div className='grid grid-cols-12 gap-3'>
        <Card className='col-span-12 md:col-span-6 xl:col-span-4'>
          <CardHeader>
            <CardTitle>In-progress jobs</CardTitle>
            <CardDescription>3 running</CardDescription>
          </CardHeader>
          <CardBody className='flex flex-col gap-3'>
            <JobRow title='Scrub · pool bulk' sub='4.2 TB / 23 TB · 18% · 2h 40m' pct={18} tone='accent' />
            <JobRow
              title='Replication · photos → offsite'
              sub='612 MB / 1.4 GB · 43% · 2m'
              pct={43}
              tone='ok'
            />
            <JobRow
              title='Snapshot prune · family-media'
              sub='schedule retention · running'
              pct={72}
              tone='warn'
            />
          </CardBody>
        </Card>

        <Card className='col-span-12 md:col-span-6 xl:col-span-4'>
          <CardHeader>
            <CardTitle>System services</CardTitle>
            <CardDescription>novanas-system</CardDescription>
          </CardHeader>
          <CardBody className='flex flex-col gap-2'>
            {[
              ['novanas-api', 'Running'],
              ['novanas-operators', 'Running'],
              ['chunk-engine (SPDK)', 'Running'],
              ['keycloak', 'Running'],
              ['openbao', 'Unsealed'],
              ['postgres', 'Running'],
              ['novaedge', 'Running'],
              ['prometheus / loki / tempo', 'Running'],
            ].map(([n, s]) => (
              <div key={n} className='flex items-center justify-between py-0.5'>
                <div className='flex items-center gap-2'>
                  <StatusDot tone='ok' />
                  <span className='mono text-sm'>{n}</span>
                </div>
                <span className='mono text-xs text-foreground-subtle'>{s}</span>
              </div>
            ))}
          </CardBody>
        </Card>

        <Card className='col-span-12 xl:col-span-4'>
          <CardHeader>
            <CardTitle>Protection summary</CardTitle>
            <CardActions>
              <Activity size={14} className='text-foreground-subtle' />
            </CardActions>
          </CardHeader>
          <CardBody className='flex flex-col gap-3'>
            <SummaryRow label='Snapshots (24h)' value='142' />
            <SummaryRow label='Replication targets' value='3' />
            <SummaryRow label='Cloud backup last run' value='14:00 · ok' tone='ok' />
            <SummaryRow label='Spare disks' value='1 (slot 8)' />
            <div className='text-xs text-foreground-subtle flex items-center gap-1.5'>
              <Shield size={12} /> All pools meet their declared protection policy.
            </div>
          </CardBody>
        </Card>
      </div>
    </>
  );
}

function JobRow({
  title,
  sub,
  pct,
  tone = 'accent',
}: {
  title: string;
  sub: string;
  pct: number;
  tone?: 'accent' | 'ok' | 'warn' | 'err';
}) {
  return (
    <div className='flex flex-col gap-1'>
      <div className='flex items-center justify-between'>
        <div className='text-sm font-medium text-foreground'>{title}</div>
        <div className='mono text-xs text-foreground-subtle tnum'>{pct}%</div>
      </div>
      <Progress value={pct} tone={tone} />
      <div className='text-xs text-foreground-subtle'>{sub}</div>
    </div>
  );
}

function SummaryRow({
  label,
  value,
  tone,
}: {
  label: string;
  value: string;
  tone?: 'ok' | 'warn' | 'err';
}) {
  return (
    <div className='flex items-center justify-between text-sm'>
      <span className='text-foreground-muted'>{label}</span>
      <span
        className={
          tone === 'ok'
            ? 'text-success mono'
            : tone === 'warn'
              ? 'text-warning mono'
              : tone === 'err'
                ? 'text-danger mono'
                : 'text-foreground mono'
        }
      >
        {value}
      </span>
    </div>
  );
}
