import { cn } from '@/lib/cn';
import type { StatusTone } from '@/types';

const TONES: Record<StatusTone, string> = {
  ok: 'bg-success shadow-[0_0_0_2px_var(--bg-2),0_0_10px_var(--ok)]',
  warn: 'bg-warning shadow-[0_0_0_2px_var(--bg-2),0_0_10px_var(--warn)]',
  err: 'bg-danger shadow-[0_0_0_2px_var(--bg-2),0_0_10px_var(--err)]',
  info: 'bg-info shadow-[0_0_0_2px_var(--bg-2)]',
  idle: 'bg-foreground-faint',
};

export function StatusDot({
  tone = 'ok',
  className,
}: {
  tone?: StatusTone;
  className?: string;
}) {
  return <span className={cn('inline-block h-2 w-2 rounded-full', TONES[tone], className)} />;
}
