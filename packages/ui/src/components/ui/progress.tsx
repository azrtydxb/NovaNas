import { cn } from '@/lib/cn';
import * as ProgressPrimitive from '@radix-ui/react-progress';
import { forwardRef } from 'react';

export const Progress = forwardRef<
  React.ElementRef<typeof ProgressPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof ProgressPrimitive.Root> & {
    tone?: 'accent' | 'warn' | 'err' | 'ok';
  }
>(({ className, value, tone = 'accent', ...props }, ref) => {
  const toneClass =
    tone === 'warn'
      ? 'bg-warning'
      : tone === 'err'
        ? 'bg-danger'
        : tone === 'ok'
          ? 'bg-success'
          : 'bg-accent';
  return (
    <ProgressPrimitive.Root
      ref={ref}
      className={cn('relative h-1 w-full overflow-hidden rounded-full bg-elevated', className)}
      {...props}
    >
      <ProgressPrimitive.Indicator
        className={cn('h-full w-full transition-all', toneClass)}
        style={{ transform: `translateX(-${100 - (value ?? 0)}%)` }}
      />
    </ProgressPrimitive.Root>
  );
});
Progress.displayName = 'Progress';
