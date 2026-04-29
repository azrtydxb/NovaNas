import { api } from "./client";

export type ReplicationJob = {
  id: string;
  name?: string;
  source?: string;
  target?: string;
  schedule?: string;
  state?: string;
  enabled?: boolean;
  lastRun?: string;
  lastBytes?: number;
  lastDur?: string;
  lastDuration?: string;
  error?: string;
};

export type ReplicationRun = {
  id: string;
  jobId?: string;
  state?: string;
  startedAt?: string;
  finishedAt?: string;
  bytes?: number;
  error?: string;
};

export type ReplicationTarget = {
  id: string;
  name?: string;
  protocol?: string;
  host?: string;
  port?: number;
  ssh_user?: string;
  region?: string;
  bucket?: string;
};

export type ReplicationSchedule = {
  id: string;
  name?: string;
  jobIds?: string[];
  cron?: string;
  enabled?: boolean;
};

export type SnapshotSchedule = {
  id: string;
  name?: string;
  datasets?: string[];
  cron?: string;
  keep?: number;
  enabled?: boolean;
};

export type ScrubPolicy = {
  id: string;
  name?: string;
  pools?: string[];
  cron?: string;
  priority?: string;
  builtin?: boolean;
};

const j = (b: unknown): RequestInit => ({
  method: "POST",
  body: JSON.stringify(b ?? {}),
});
const put = (b: unknown): RequestInit => ({
  method: "PUT",
  body: JSON.stringify(b ?? {}),
});

export const replication = {
  // Jobs (read-only via this surface; create/delete via schedules?)
  listJobs: () => api<ReplicationJob[]>(`/api/v1/replication-jobs`),
  getJob: (id: string) =>
    api<ReplicationJob>(`/api/v1/replication-jobs/${encodeURIComponent(id)}`),
  runJob: (id: string) =>
    api<unknown>(`/api/v1/replication-jobs/${encodeURIComponent(id)}/run`, {
      method: "POST",
    }),
  listRuns: (id: string) =>
    api<ReplicationRun[]>(`/api/v1/replication-jobs/${encodeURIComponent(id)}/runs`),
  // TODO: backend missing — POST /replication-jobs (create), PUT /replication-jobs/{id},
  // DELETE /replication-jobs/{id}. Jobs are derived from replication-schedules.

  // Targets
  listTargets: () =>
    api<ReplicationTarget[]>(`/api/v1/scheduler/replication-targets`),
  getTarget: (id: string) =>
    api<ReplicationTarget>(
      `/api/v1/scheduler/replication-targets/${encodeURIComponent(id)}`
    ),
  createTarget: (body: Partial<ReplicationTarget>) =>
    api<ReplicationTarget>(`/api/v1/scheduler/replication-targets`, j(body)),
  updateTarget: (id: string, body: Partial<ReplicationTarget>) =>
    api<ReplicationTarget>(
      `/api/v1/scheduler/replication-targets/${encodeURIComponent(id)}`,
      put(body)
    ),
  deleteTarget: (id: string) =>
    api<unknown>(`/api/v1/scheduler/replication-targets/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  // Replication schedules
  listReplicationSchedules: () =>
    api<ReplicationSchedule[]>(`/api/v1/scheduler/replication-schedules`),
  getReplicationSchedule: (id: string) =>
    api<ReplicationSchedule>(
      `/api/v1/scheduler/replication-schedules/${encodeURIComponent(id)}`
    ),
  createReplicationSchedule: (body: Partial<ReplicationSchedule>) =>
    api<ReplicationSchedule>(`/api/v1/scheduler/replication-schedules`, j(body)),
  updateReplicationSchedule: (id: string, body: Partial<ReplicationSchedule>) =>
    api<ReplicationSchedule>(
      `/api/v1/scheduler/replication-schedules/${encodeURIComponent(id)}`,
      put(body)
    ),
  deleteReplicationSchedule: (id: string) =>
    api<unknown>(
      `/api/v1/scheduler/replication-schedules/${encodeURIComponent(id)}`,
      { method: "DELETE" }
    ),

  // Snapshot schedules
  listSnapshotSchedules: () =>
    api<SnapshotSchedule[]>(`/api/v1/scheduler/snapshot-schedules`),
  getSnapshotSchedule: (id: string) =>
    api<SnapshotSchedule>(
      `/api/v1/scheduler/snapshot-schedules/${encodeURIComponent(id)}`
    ),
  createSnapshotSchedule: (body: Partial<SnapshotSchedule>) =>
    api<SnapshotSchedule>(`/api/v1/scheduler/snapshot-schedules`, j(body)),
  updateSnapshotSchedule: (id: string, body: Partial<SnapshotSchedule>) =>
    api<SnapshotSchedule>(
      `/api/v1/scheduler/snapshot-schedules/${encodeURIComponent(id)}`,
      put(body)
    ),
  deleteSnapshotSchedule: (id: string) =>
    api<unknown>(`/api/v1/scheduler/snapshot-schedules/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),

  // Scrub policies
  listScrubPolicies: () => api<ScrubPolicy[]>(`/api/v1/scrub-policies`),
  getScrubPolicy: (id: string) =>
    api<ScrubPolicy>(`/api/v1/scrub-policies/${encodeURIComponent(id)}`),
  createScrubPolicy: (body: Partial<ScrubPolicy>) =>
    api<ScrubPolicy>(`/api/v1/scrub-policies`, j(body)),
  updateScrubPolicy: (id: string, body: Partial<ScrubPolicy>) =>
    api<ScrubPolicy>(`/api/v1/scrub-policies/${encodeURIComponent(id)}`, put(body)),
  deleteScrubPolicy: (id: string) =>
    api<unknown>(`/api/v1/scrub-policies/${encodeURIComponent(id)}`, {
      method: "DELETE",
    }),
};
