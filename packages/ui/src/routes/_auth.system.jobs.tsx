// TODO(i18n-wave-12): strings on this page are still raw English. Migrate to <Trans>/i18n._() once wave 12 is green.
import { type JobListFilters, type JobState, useCancelJob, useJobs } from '@/api/jobs';
import { EmptyState } from '@/components/common/empty-state';
import { PageHeader } from '@/components/common/page-header';
import { StatusDot } from '@/components/common/status-dot';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Progress } from '@/components/ui/progress';
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
import { useToast } from '@/hooks/use-toast';
import type { StatusTone } from '@/types';
import { createFileRoute } from '@tanstack/react-router';
import { Activity, X } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/system/jobs')({
  component: JobsPage,
});

const stateOptions: { value: JobState | ''; label: string }[] = [
  { value: '', label: 'All states' },
  { value: 'pending', label: 'Pending' },
  { value: 'running', label: 'Running' },
  { value: 'succeeded', label: 'Succeeded' },
  { value: 'failed', label: 'Failed' },
  { value: 'canceled', label: 'Canceled' },
];

function toneForState(state: JobState): StatusTone {
  switch (state) {
    case 'succeeded':
      return 'ok';
    case 'failed':
      return 'err';
    case 'running':
    case 'cancelRequested':
      return 'warn';
    case 'canceled':
      return 'idle';
    default:
      return 'info';
  }
}

function JobsPage() {
  const [state, setState] = useState<JobState | ''>('');
  const [kind, setKind] = useState('');
  const [owner, setOwner] = useState('');

  const filters: JobListFilters = {};
  if (state) filters.state = state;
  if (kind) filters.kind = kind;
  if (owner) filters.owner = owner;

  const jobs = useJobs(filters);
  const cancel = useCancelJob();
  const toast = useToast();

  return (
    <>
      <PageHeader
        title='Jobs'
        subtitle='Long-running background tasks (installs, snapshots, replication, etc.).'
      />

      <div className='flex flex-wrap items-end gap-2 mb-3'>
        <div className='flex flex-col gap-1 text-xs'>
          <span className='text-foreground-subtle'>State</span>
          <Select value={state} onValueChange={(v) => setState(v as JobState | '')}>
            <SelectTrigger className='w-40' aria-label='State'>
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              {stateOptions.map((o) => (
                <SelectItem key={o.value || 'all'} value={o.value}>
                  {o.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        </div>
        <div className='flex flex-col gap-1 text-xs'>
          <span className='text-foreground-subtle'>Kind</span>
          <Input
            className='w-48'
            placeholder='e.g. install-app'
            value={kind}
            onChange={(e) => setKind(e.target.value)}
            aria-label='Kind'
          />
        </div>
        <div className='flex flex-col gap-1 text-xs'>
          <span className='text-foreground-subtle'>Owner</span>
          <Input
            className='w-40'
            placeholder='user'
            value={owner}
            onChange={(e) => setOwner(e.target.value)}
            aria-label='Owner'
          />
        </div>
        <Button variant='ghost' size='sm' onClick={() => jobs.refetch()}>
          Refresh
        </Button>
      </div>

      {jobs.isLoading ? (
        <div className='flex flex-col gap-2'>
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className='h-9' />
          ))}
        </div>
      ) : jobs.isError ? (
        <EmptyState
          icon={<Activity size={28} />}
          title='Unable to load jobs'
          description={(jobs.error as Error)?.message ?? 'Try again in a moment.'}
          action={<Button onClick={() => jobs.refetch()}>Retry</Button>}
        />
      ) : (jobs.data?.length ?? 0) === 0 ? (
        <EmptyState icon={<Activity size={28} />} title='No jobs match' />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>Title</TableHeaderCell>
                <TableHeaderCell>Kind</TableHeaderCell>
                <TableHeaderCell>State</TableHeaderCell>
                <TableHeaderCell>Progress</TableHeaderCell>
                <TableHeaderCell>Owner</TableHeaderCell>
                <TableHeaderCell>Started</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {jobs.data!.map((j) => {
                const cancellable = j.state === 'pending' || j.state === 'running';
                return (
                  <TableRow key={j.id}>
                    <TableCell>
                      <StatusDot tone={toneForState(j.state)} className='mr-2' />
                      <span className='text-foreground'>{j.title ?? j.id}</span>
                    </TableCell>
                    <TableCell className='mono text-xs'>{j.kind}</TableCell>
                    <TableCell>
                      <Badge>{j.state}</Badge>
                    </TableCell>
                    <TableCell className='min-w-[120px]'>
                      {typeof j.progress === 'number' ? (
                        <Progress value={j.progress} />
                      ) : (
                        <span className='text-foreground-subtle text-xs'>—</span>
                      )}
                    </TableCell>
                    <TableCell className='mono text-xs'>{j.owner ?? '—'}</TableCell>
                    <TableCell className='mono text-xs text-foreground-muted'>
                      {j.startedAt ?? j.createdAt}
                    </TableCell>
                    <TableCell className='text-right'>
                      {cancellable && (
                        <Button
                          size='sm'
                          variant='ghost'
                          title='Cancel'
                          disabled={cancel.isPending}
                          onClick={async () => {
                            try {
                              await cancel.mutateAsync(j.id);
                              toast.success('Cancel requested', j.id);
                            } catch (err) {
                              toast.error('Cancel failed', (err as Error)?.message);
                            }
                          }}
                        >
                          <X size={11} />
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}
    </>
  );
}
