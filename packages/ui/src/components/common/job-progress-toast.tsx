import { type Job, isTerminal, useJob } from '@/api/jobs';
/**
 * Floating job-progress toasts (issue #6).
 *
 * Rendered once at the app root. For each tracked job id we mount a
 * <JobProgressToast/> that subscribes to useJob and updates in-place over
 * WebSocket. On success, the toast auto-dismisses after a short delay.
 * On failure, it stays pinned with a "View details" link.
 */
import { Button } from '@/components/ui/button';
import { Progress } from '@/components/ui/progress';
import { useJobsUiStore } from '@/stores/jobs';
import { Link } from '@tanstack/react-router';
import { X } from 'lucide-react';
import { useEffect } from 'react';

export function JobProgressToaster() {
  const tracked = useJobsUiStore((s) => s.tracked);
  if (tracked.length === 0) return null;
  return (
    <div className='fixed bottom-4 right-4 z-50 flex flex-col gap-2 w-[340px] pointer-events-none'>
      {tracked.map((t) => (
        <JobProgressToast key={t.id} jobId={t.id} title={t.title} />
      ))}
    </div>
  );
}

export function JobProgressToast({ jobId, title }: { jobId: string; title: string }) {
  const q = useJob(jobId);
  const untrack = useJobsUiStore((s) => s.untrack);
  const job = q.data;

  useEffect(() => {
    if (!job) return;
    if (job.state === 'succeeded' || job.state === 'canceled') {
      const t = setTimeout(() => untrack(jobId), 3_000);
      return () => clearTimeout(t);
    }
    return undefined;
  }, [job?.state, jobId, untrack, job]);

  return (
    <div className='pointer-events-auto bg-panel border border-border rounded-md shadow-lg p-3 text-sm flex flex-col gap-2'>
      <div className='flex items-start gap-2'>
        <div className='flex-1'>
          <div className='font-medium text-foreground'>{title}</div>
          <div className='text-xs text-foreground-subtle mono'>
            {job ? jobStateLabel(job) : 'Queued…'}
          </div>
        </div>
        <button
          type='button'
          className='text-foreground-subtle hover:text-foreground'
          aria-label='Dismiss'
          onClick={() => untrack(jobId)}
        >
          <X size={12} />
        </button>
      </div>
      {job && !isTerminal(job.state) && (
        <Progress value={typeof job.progress === 'number' ? job.progress : undefined} />
      )}
      {job?.state === 'failed' && (
        <div className='flex items-center justify-between'>
          <span className='text-xs text-danger'>{job.error ?? job.message ?? 'Job failed'}</span>
          <Link to='/system/jobs'>
            <Button size='sm' variant='ghost'>
              View details
            </Button>
          </Link>
        </div>
      )}
    </div>
  );
}

function jobStateLabel(job: Job): string {
  switch (job.state) {
    case 'pending':
      return 'Pending';
    case 'running':
      return job.progress != null
        ? `Running… ${Math.round(job.progress)}%`
        : (job.message ?? 'Running…');
    case 'succeeded':
      return 'Succeeded';
    case 'failed':
      return 'Failed';
    case 'canceled':
      return 'Canceled';
    case 'cancelRequested':
      return 'Canceling…';
    default:
      return job.state;
  }
}
