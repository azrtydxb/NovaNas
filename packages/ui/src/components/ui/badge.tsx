import { cn } from '@/lib/cn';
import { type VariantProps, cva } from 'class-variance-authority';
import type { HTMLAttributes } from 'react';

const badgeVariants = cva(
  'inline-flex items-center gap-1.5 rounded-full px-2 h-[22px] text-xs border whitespace-nowrap',
  {
    variants: {
      tone: {
        default: 'bg-elevated text-foreground border-border',
        ok: 'bg-success-soft text-success border-transparent',
        warn: 'bg-warning-soft text-warning border-transparent',
        err: 'bg-danger-soft text-danger border-transparent',
        accent: 'bg-accent-soft text-accent border-transparent',
      },
    },
    defaultVariants: { tone: 'default' },
  }
);

export interface BadgeProps
  extends HTMLAttributes<HTMLSpanElement>,
    VariantProps<typeof badgeVariants> {
  dot?: boolean;
}

export function Badge({ className, tone, dot, children, ...props }: BadgeProps) {
  return (
    <span className={cn(badgeVariants({ tone }), className)} {...props}>
      {dot && <span className='h-1.5 w-1.5 rounded-full bg-current' />}
      {children}
    </span>
  );
}
