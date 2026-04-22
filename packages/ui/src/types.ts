/**
 * Shared UI-local types. Domain types are imported from @novanas/schemas.
 */

export type StatusTone = 'ok' | 'warn' | 'err' | 'info' | 'idle';

export interface NavItem {
  id: string;
  label: string;
  to: string;
  icon?: string;
  count?: number;
  roles?: readonly ('admin' | 'user')[];
  children?: NavItem[];
}

export interface HealthSummary {
  status: StatusTone;
  label: string;
  capacity: { used: number; total: number };
  appsRunning: number;
  appsInstalled: number;
  vmsRunning: number;
  vmsDefined: number;
  disksActive: number;
  poolsOnline: number;
  lastScrubAgo: string;
  lastConfigBackup: string;
}
