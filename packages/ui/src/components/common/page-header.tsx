import { cn } from '@/lib/cn';
import type { ReactNode } from 'react';

export interface PageHeaderProps {
  title: string;
  subtitle?: ReactNode;
  actions?: ReactNode;
  className?: string;
}

export function PageHeader({ title, subtitle, actions, className }: PageHeaderProps) {
  return (
    <div
      className={cn(
        'flex items-end justify-between gap-4 pb-3.5 mb-3.5 border-b border-border',
        className
      )}
    >
      <div>
        <h1 className='text-2xl font-semibold text-foreground tracking-tight leading-tight'>
          {title}
        </h1>
        {subtitle && <div className='text-foreground-subtle text-sm mt-1'>{subtitle}</div>}
      </div>
      {actions && <div className='flex gap-2'>{actions}</div>}
    </div>
  );
}
