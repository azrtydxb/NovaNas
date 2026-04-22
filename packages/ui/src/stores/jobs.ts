/**
 * Tracked long-running jobs (issue #6).
 *
 * When a composite mutation (install-app, create-vm, snapshot, etc.) returns
 * a job id, callers register it here with `trackJob(id, title)`. A global
 * `<JobProgressToaster />` mounted by the app shell subscribes to each
 * tracked job and renders a progress toast that updates over WebSocket.
 *
 * Succeeded jobs auto-dismiss after a short delay; failed jobs stay pinned
 * with a "View details" link to /system/jobs.
 */
import { create } from 'zustand';

export interface TrackedJob {
  id: string;
  title: string;
}

interface JobsUiState {
  tracked: TrackedJob[];
  track: (j: TrackedJob) => void;
  untrack: (id: string) => void;
  clear: () => void;
}

export const useJobsUiStore = create<JobsUiState>((set) => ({
  tracked: [],
  track: (j) =>
    set((s) => (s.tracked.some((t) => t.id === j.id) ? s : { tracked: [...s.tracked, j] })),
  untrack: (id) => set((s) => ({ tracked: s.tracked.filter((t) => t.id !== id) })),
  clear: () => set({ tracked: [] }),
}));

export function trackJob(id: string, title: string): void {
  useJobsUiStore.getState().track({ id, title });
}

/**
 * Best-effort extractor for a job id returned from a composite mutation.
 * The API may return `{ jobId }`, `{ job: { id } }`, or embed it in headers
 * (not handled here — the fetch client would need to surface those).
 */
export function extractJobId(resp: unknown): string | undefined {
  if (!resp || typeof resp !== 'object') return undefined;
  const r = resp as Record<string, unknown>;
  if (typeof r.jobId === 'string') return r.jobId;
  if (r.job && typeof r.job === 'object') {
    const job = r.job as { id?: unknown };
    if (typeof job.id === 'string') return job.id;
  }
  if (r.metadata && typeof r.metadata === 'object') {
    const md = r.metadata as { annotations?: Record<string, string> };
    const ann = md.annotations?.['novanas.io/job-id'];
    if (ann) return ann;
  }
  return undefined;
}

/** Convenience: track a job from a mutation response if an id is extractable. */
export function maybeTrackJobFromResponse(resp: unknown, title: string): void {
  const id = extractJobId(resp);
  if (id) trackJob(id, title);
}
