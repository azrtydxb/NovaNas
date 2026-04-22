import { formatDistanceToNowStrict } from 'date-fns';

const UNITS = ['B', 'KB', 'MB', 'GB', 'TB', 'PB', 'EB'] as const;

export function formatBytes(bytes: number | null | undefined, digits = 1): string {
  if (bytes == null || Number.isNaN(bytes)) return '—';
  if (bytes === 0) return '0 B';
  const neg = bytes < 0;
  let b = Math.abs(bytes);
  let i = 0;
  while (b >= 1000 && i < UNITS.length - 1) {
    b /= 1000;
    i++;
  }
  const val = b.toFixed(i === 0 ? 0 : digits);
  return `${neg ? '-' : ''}${val} ${UNITS[i]}`;
}

export function formatNumber(n: number, digits = 1): string {
  if (Math.abs(n) >= 1_000_000) return `${(n / 1_000_000).toFixed(digits)}M`;
  if (Math.abs(n) >= 1_000) return `${(n / 1_000).toFixed(digits)}k`;
  return String(Math.round(n));
}

export function formatPct(v: number, digits = 0): string {
  return `${(v * 100).toFixed(digits)}%`;
}

export function formatRelativeTime(date: Date | string | number): string {
  const d = typeof date === 'object' ? date : new Date(date);
  return formatDistanceToNowStrict(d, { addSuffix: true });
}
