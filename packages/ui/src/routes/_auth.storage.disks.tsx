import { useDisk, useDisksLive, useUpdateDisk } from '@/api/disks';
import { usePools } from '@/api/pools';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { StatusDot } from '@/components/common/status-dot';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeaderCell,
  TableRow,
} from '@/components/ui/table';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { cn } from '@/lib/cn';
import { formatBytes } from '@/lib/format';
import type { StatusTone } from '@/types';
import type { Disk } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { HardDrive } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/storage/disks')({
  component: DisksPage,
});

function smartTone(d: Disk): StatusTone {
  const h = d.status?.smart?.overallHealth;
  if (h === 'FAIL') return 'err';
  if (h === 'WARN') return 'warn';
  const st = d.status?.state;
  if (st === 'FAILED' || st === 'QUARANTINED') return 'err';
  if (st === 'DEGRADED' || st === 'DRAINING') return 'warn';
  if (st === 'ACTIVE' || st === 'ASSIGNED') return 'ok';
  return 'idle';
}

function DisksPage() {
  const disks = useDisksLive();
  const [view, setView] = useState<'grid' | 'list'>('grid');
  const [selectedWwn, setSelectedWwn] = useState<string | null>(null);

  return (
    <>
      <PageHeader
        title='Disks'
        subtitle='Physical devices and their SMART state, updated live.'
        actions={
          <div className='flex gap-1 border border-border rounded-md p-0.5'>
            <Button
              size='sm'
              variant={view === 'grid' ? 'primary' : 'ghost'}
              onClick={() => setView('grid')}
            >
              Grid
            </Button>
            <Button
              size='sm'
              variant={view === 'list' ? 'primary' : 'ghost'}
              onClick={() => setView('list')}
            >
              List
            </Button>
          </div>
        }
      />

      {disks.isLoading ? (
        <div className='grid grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-3'>
          {[0, 1, 2, 3, 4, 5].map((i) => (
            <Skeleton key={i} className='h-28' />
          ))}
        </div>
      ) : disks.isError ? (
        <EmptyState
          icon={<HardDrive size={28} />}
          title='Unable to load disks'
          description={(disks.error as Error)?.message ?? 'Try again in a moment.'}
          action={<Button onClick={() => disks.refetch()}>Retry</Button>}
        />
      ) : (disks.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<HardDrive size={28} />}
          title='No disks detected'
          description='Connect disks to this node and they will show up here.'
        />
      ) : view === 'grid' ? (
        <div className='grid grid-cols-2 md:grid-cols-3 xl:grid-cols-5 gap-3'>
          {disks.data!.map((d) => (
            <DiskCard
              key={d.metadata.name}
              disk={d}
              onClick={() => setSelectedWwn(d.status?.wwn ?? d.metadata.name)}
            />
          ))}
        </div>
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>Slot</TableHeaderCell>
                <TableHeaderCell>Model</TableHeaderCell>
                <TableHeaderCell>Class</TableHeaderCell>
                <TableHeaderCell>Size</TableHeaderCell>
                <TableHeaderCell>Pool</TableHeaderCell>
                <TableHeaderCell>State</TableHeaderCell>
                <TableHeaderCell>SMART</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {disks.data!.map((d) => (
                <TableRow
                  key={d.metadata.name}
                  className='cursor-pointer'
                  onClick={() => setSelectedWwn(d.status?.wwn ?? d.metadata.name)}
                >
                  <TableCell className='mono'>{d.status?.slot ?? '—'}</TableCell>
                  <TableCell className='mono text-xs'>{d.status?.model ?? '—'}</TableCell>
                  <TableCell>
                    <Badge>{d.status?.deviceClass ?? 'unknown'}</Badge>
                    {d.metadata?.labels?.['novanas.io/system'] === 'true' && (
                      <Badge
                        tone='warn'
                        className='ml-1'
                        title='OS disk — cannot be added to a pool'
                      >
                        System
                      </Badge>
                    )}
                  </TableCell>
                  <TableCell className='mono'>
                    {d.status?.sizeBytes ? formatBytes(d.status.sizeBytes) : '—'}
                  </TableCell>
                  <TableCell className='mono text-xs'>{d.spec.pool ?? '—'}</TableCell>
                  <TableCell>
                    <StatusDot tone={smartTone(d)} className='mr-2' />
                    <span className='mono text-xs text-foreground-muted'>
                      {d.status?.state ?? 'UNKNOWN'}
                    </span>
                  </TableCell>
                  <TableCell className='mono text-xs text-foreground-muted'>
                    {d.status?.smart?.overallHealth ?? '—'}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <DiskDetailDialog
        wwn={selectedWwn}
        onClose={() => setSelectedWwn(null)}
        fallback={disks.data?.find((d) => (d.status?.wwn ?? d.metadata.name) === selectedWwn)}
      />
    </>
  );
}

function DiskCard({ disk, onClick }: { disk: Disk; onClick: () => void }) {
  const tone = smartTone(disk);
  return (
    <button
      type='button'
      onClick={onClick}
      className={cn(
        'relative text-left bg-panel border border-border rounded-md p-3 hover:border-border-strong transition-colors',
        'flex flex-col gap-1.5'
      )}
    >
      <div className='flex items-center gap-2'>
        <StatusDot tone={tone} />
        <span className='mono text-sm text-foreground'>slot {disk.status?.slot ?? '—'}</span>
        {disk.metadata?.labels?.['novanas.io/system'] === 'true' && (
          <Badge tone='warn' title='OS disk — cannot be added to a pool'>
            System
          </Badge>
        )}
        <span className='ml-auto text-2xs uppercase tracking-wider text-foreground-subtle'>
          {disk.status?.deviceClass ?? '—'}
        </span>
      </div>
      <div className='text-xs text-foreground-muted mono truncate'>{disk.status?.model ?? '—'}</div>
      <div className='mono text-md text-foreground'>
        {disk.status?.sizeBytes ? formatBytes(disk.status.sizeBytes) : '—'}
      </div>
      <div className='flex items-center justify-between text-xs text-foreground-subtle'>
        <span>{disk.spec.pool ? `pool: ${disk.spec.pool}` : 'unassigned'}</span>
        <span className='mono'>{disk.status?.state ?? 'UNKNOWN'}</span>
      </div>
    </button>
  );
}

function DiskDetailDialog({
  wwn,
  onClose,
  fallback,
}: {
  wwn: string | null;
  onClose: () => void;
  fallback?: Disk;
}) {
  const { canMutate } = useAuth();
  const mayMutate = canMutate();
  const toast = useToast();
  const open = !!wwn;
  const disk = useDisk(open ? (wwn ?? undefined) : undefined);
  const data: Disk | undefined = disk.data ?? fallback;
  const pools = usePools();
  const update = useUpdateDisk(wwn ?? '');
  const [pool, setPool] = useState<string>('');

  const currentPool = data?.spec.pool ?? '';
  if (open && pool !== currentPool && update.isIdle) {
    setPool(currentPool);
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Disk · slot {data?.status?.slot ?? '—'}</DialogTitle>
          <DialogDescription>
            {data?.status?.model ?? '—'} · {data?.status?.serial ?? ''}
          </DialogDescription>
        </DialogHeader>

        <div className='flex flex-col gap-3 text-sm'>
          <div className='grid grid-cols-2 gap-2 mono text-xs'>
            <div>
              <div className='text-foreground-subtle uppercase tracking-wider'>State</div>
              <div>{data?.status?.state ?? '—'}</div>
            </div>
            <div>
              <div className='text-foreground-subtle uppercase tracking-wider'>Class</div>
              <div>{data?.status?.deviceClass ?? '—'}</div>
            </div>
            <div>
              <div className='text-foreground-subtle uppercase tracking-wider'>Size</div>
              <div>{data?.status?.sizeBytes ? formatBytes(data.status.sizeBytes) : '—'}</div>
            </div>
            <div>
              <div className='text-foreground-subtle uppercase tracking-wider'>WWN</div>
              <div className='truncate'>{data?.status?.wwn ?? '—'}</div>
            </div>
          </div>

          <div className='border border-border rounded-md p-2 text-xs flex flex-col gap-0.5 mono'>
            <div className='uppercase tracking-wider text-foreground-subtle mb-1'>SMART</div>
            <div>health: {data?.status?.smart?.overallHealth ?? '—'}</div>
            <div>temp: {data?.status?.smart?.temperature ?? '—'}°C</div>
            <div>power-on hours: {data?.status?.smart?.powerOnHours ?? '—'}</div>
            <div>reallocated: {data?.status?.smart?.reallocatedSectors ?? 0}</div>
            <div>pending: {data?.status?.smart?.pendingSectors ?? 0}</div>
          </div>

          <FormField label='Assign to pool' hint='Leave empty to mark the disk as unassigned.'>
            <Select value={pool} onValueChange={setPool} disabled={!mayMutate}>
              <SelectTrigger>
                <SelectValue placeholder='Unassigned' />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='__none__'>Unassigned</SelectItem>
                {pools.data?.map((p) => (
                  <SelectItem key={p.metadata.name} value={p.metadata.name}>
                    {p.metadata.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormField>
        </div>

        <DialogFooter>
          <Button variant='ghost' onClick={onClose}>
            Close
          </Button>
          {mayMutate && (
            <Button
              variant='primary'
              disabled={pool === currentPool || update.isPending}
              onClick={async () => {
                try {
                  const nextPool = pool === '__none__' ? undefined : pool || undefined;
                  await update.mutateAsync({ spec: { pool: nextPool } });
                  toast.success('Disk updated', data?.status?.slot ?? wwn ?? '');
                  onClose();
                } catch (err) {
                  toast.error('Failed to update disk', (err as Error)?.message);
                }
              }}
            >
              {update.isPending ? 'Saving…' : 'Save'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
