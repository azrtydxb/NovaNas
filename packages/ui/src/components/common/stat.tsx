import { cn } from '@/lib/cn';
import { Sparkline } from './sparkline';

export interface StatProps {
  label: string;
  value: React.ReactNode;
  unit?: string;
  delta?: string;
  up?: boolean;
  down?: boolean;
  data?: number[];
  color?: string;
  className?: string;
}

export function Stat({ label, value, unit, delta, up, down, data, color = 'var(--accent)', className }: StatProps) {
  return (
    <div
      className={cn(
        'relative overflow-hidden rounded-md border border-border bg-panel px-3.5 py-3 min-h-[96px] flex flex-col gap-1.5',
        className
      )}
    >
      <div className='text-2xs uppercase tracking-wider text-foreground-subtle flex items-center gap-1.5'>
        {label}
      </div>
      <div className='mono text-[24px] font-medium text-foreground tracking-tight tnum'>
        {value}
        {unit && <span className='text-base text-foreground-subtle ml-1'>{unit}</span>}
      </div>
      {delta && (
        <div
          className={cn(
            'text-xs',
            up && 'text-success',
            down && 'text-danger',
            !up && !down && 'text-foreground-muted'
          )}
        >
          {delta} <span className='text-foreground-subtle'>· 1h</span>
        </div>
      )}
      {data && (
        <div className='absolute inset-x-0 bottom-0 h-10 opacity-80 pointer-events-none'>
          <Sparkline data={data} color={color} height={40} />
        </div>
      )}
    </div>
  );
}
