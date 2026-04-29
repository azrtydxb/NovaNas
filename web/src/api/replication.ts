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

export const replication = {
  listJobs: () => api<ReplicationJob[]>(`/api/v1/replication-jobs`),
  getJob: (id: string) =>
    api<ReplicationJob>(`/api/v1/replication-jobs/${encodeURIComponent(id)}`),
  runJob: (id: string) =>
    api<unknown>(`/api/v1/replication-jobs/${encodeURIComponent(id)}/run`, {
      method: "POST",
    }),
  listRuns: (id: string) =>
    api<ReplicationRun[]>(`/api/v1/replication-jobs/${encodeURIComponent(id)}/runs`),

  listTargets: () =>
    api<ReplicationTarget[]>(`/api/v1/scheduler/replication-targets`),
  listReplicationSchedules: () =>
    api<ReplicationSchedule[]>(`/api/v1/scheduler/replication-schedules`),
  listSnapshotSchedules: () =>
    api<SnapshotSchedule[]>(`/api/v1/scheduler/snapshot-schedules`),
  listScrubPolicies: () => api<ScrubPolicy[]>(`/api/v1/scrub-policies`),
};
