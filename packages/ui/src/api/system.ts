import { useQuery } from '@tanstack/react-query';
import { QUERY_DEFAULTS, api } from './client';

export type HealthStatus = 'ok' | 'warn' | 'err';

export interface SystemHealth {
  status: HealthStatus;
  message?: string;
  capacity: { usedBytes: number; totalBytes: number };
  pools: { online: number; total: number };
  disks: { active: number; total: number };
  apps: { running: number; installed: number };
  vms: { running: number; defined: number };
  services: Array<{ name: string; status: string; tone?: HealthStatus }>;
  lastScrubAt?: string;
  lastConfigBackupAt?: string;
}

export interface SystemInfo {
  hostname: string;
  version: string;
  channel?: string;
  uptimeSeconds: number;
  timezone: string;
  nodeId?: string;
}

export function useSystemHealth() {
  return useQuery<SystemHealth>({
    queryKey: ['system', 'health'],
    queryFn: () => api.get<SystemHealth>('/system/health'),
    ...QUERY_DEFAULTS,
    refetchInterval: 30_000,
  });
}

export function useSystemInfo() {
  return useQuery<SystemInfo>({
    queryKey: ['system', 'info'],
    queryFn: () => api.get<SystemInfo>('/system/info'),
    ...QUERY_DEFAULTS,
    staleTime: 5 * 60_000,
  });
}
