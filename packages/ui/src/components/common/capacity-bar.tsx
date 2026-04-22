import { cn } from '@/lib/cn';
import { formatBytes } from '@/lib/format';

export interface CapacityBarProps {
  used: number;
  total: number;
  label?: string;
  className?: string;
}

export function CapacityBar({ used, total, label, className }: CapacityBarProps) {
  const pct = total > 0 ? Math.min(100, (used / total) * 100) : 0;
  const tone = pct > 92 ? 'bg-danger' : pct > 80 ? 'bg-warning' : 'bg-accent';
  return (
    <div
      className={cn('grid grid-cols-[80px_1fr_80px] items-center gap-2 min-w-[220px]', className)}
    >
      <span className='mono text-xs text-foreground-muted text-left truncate'>
        {label ?? formatBytes(used)}
      </span>
      <span className='h-1.5 bg-elevated rounded-full overflow-hidden'>
        <span
          className={cn('block h-full transition-[width] duration-500', tone)}
          style={{ width: `${pct}%` }}
        />
      </span>
      <span className='mono text-xs text-foreground-muted text-right'>{formatBytes(total)}</span>
    </div>
  );
}
