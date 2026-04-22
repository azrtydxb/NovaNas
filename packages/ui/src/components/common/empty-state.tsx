import { cn } from '@/lib/cn';
import type { ReactNode } from 'react';

export interface EmptyStateProps {
  title: string;
  description?: ReactNode;
  icon?: ReactNode;
  action?: ReactNode;
  className?: string;
}

export function EmptyState({ title, description, icon, action, className }: EmptyStateProps) {
  return (
    <div
      className={cn(
        'flex flex-col items-center justify-center text-center gap-3 py-16 px-6',
        'border border-dashed border-border rounded-md bg-panel/60',
        className
      )}
    >
      {icon && <div className='text-foreground-subtle'>{icon}</div>}
      <div className='text-md font-medium text-foreground'>{title}</div>
      {description && (
        <div className='text-sm text-foreground-muted max-w-prose'>{description}</div>
      )}
      {action}
    </div>
  );
}
