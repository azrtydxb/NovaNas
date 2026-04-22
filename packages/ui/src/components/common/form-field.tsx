import { cn } from '@/lib/cn';
import type { ReactNode } from 'react';

export interface FormFieldProps {
  label: string;
  htmlFor?: string;
  hint?: ReactNode;
  error?: string;
  required?: boolean;
  children: ReactNode;
  className?: string;
}

export function FormField({
  label,
  htmlFor,
  hint,
  error,
  required,
  children,
  className,
}: FormFieldProps) {
  return (
    <div className={cn('flex flex-col gap-1', className)}>
      <label htmlFor={htmlFor} className='text-xs uppercase tracking-wider text-foreground-subtle'>
        {label}
        {required && <span className='text-danger ml-1'>*</span>}
      </label>
      {children}
      {error ? (
        <span className='text-xs text-danger'>{error}</span>
      ) : hint ? (
        <span className='text-xs text-foreground-subtle'>{hint}</span>
      ) : null}
    </div>
  );
}
