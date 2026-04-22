/**
 * Job (long-running task) API hooks (issue #6).
 *
 * The API contract (from A10-API-Infra):
 *   GET  /api/v1/jobs                 -> { items: Job[] }
 *   GET  /api/v1/jobs/:id             -> Job
 *   POST /api/v1/jobs/:id/cancel      -> Job
 *
 * WS channel:  job:{id} and job:* for list-level changes.
 *
 * There is no `Job` type in @novanas/schemas yet, so we define a minimal
 * UI-local shape. When the schema lands, it should be a drop-in replacement.
 */
import { useLiveQuery } from '@/hooks/use-live-query';
import { useMutation, useQueryClient } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api, unwrapList } from './client';

export type JobState =
  | 'pending'
  | 'running'
  | 'succeeded'
  | 'failed'
  | 'canceled'
  | 'cancelRequested';

export interface Job {
  id: string;
  kind: string;
  /** Human-readable summary, e.g. "Install app nextcloud". */
  title?: string;
  state: JobState;
  /** 0–100. May be undefined for indeterminate work. */
  progress?: number;
  message?: string;
  error?: string;
  createdAt: string;
  updatedAt?: string;
  startedAt?: string;
  finishedAt?: string;
  owner?: string;
  /** Free-form tags the server uses to correlate the job with the resource it's acting on. */
  refs?: Record<string, string>;
}

export interface JobListFilters {
  state?: JobState;
  kind?: string;
  owner?: string;
}

export const jobsKey = (filters?: JobListFilters) => ['jobs', filters ?? {}] as const;
export const jobKey = (id: string) => ['job', id] as const;

export function useJobs(filters?: JobListFilters) {
  return useLiveQuery<Job[]>(
    jobsKey(filters),
    async () =>
      unwrapList<Job>(
        await api.get('/jobs', {
          searchParams: {
            state: filters?.state,
            kind: filters?.kind,
            owner: filters?.owner,
          },
        })
      ),
    { ...QUERY_DEFAULTS, staleTime: 5_000, wsChannel: 'job:*' }
  );
}

export function useJob(id: string | undefined) {
  return useLiveQuery<Job>(
    jobKey(id ?? ''),
    () => api.get<Job>(`/jobs/${encodeURIComponent(id!)}`),
    {
      ...QUERY_DEFAULTS,
      staleTime: 2_000,
      enabled: !!id,
      wsChannel: id ? `job:${id}` : null,
    }
  );
}

export function useCancelJob() {
  const qc = useQueryClient();
  return useMutation({
    mutationFn: (id: string) => api.post<Job>(`/jobs/${encodeURIComponent(id)}/cancel`),
    onSuccess: (_data, id) => {
      qc.invalidateQueries({ queryKey: jobKey(id) });
      qc.invalidateQueries({ queryKey: ['jobs'] });
    },
  });
}

export function isTerminal(state: JobState): boolean {
  return state === 'succeeded' || state === 'failed' || state === 'canceled';
}
