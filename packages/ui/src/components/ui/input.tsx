import { cn } from '@/lib/cn';
import { type InputHTMLAttributes, forwardRef } from 'react';

export const Input = forwardRef<HTMLInputElement, InputHTMLAttributes<HTMLInputElement>>(
  ({ className, type = 'text', ...props }, ref) => (
    <input
      ref={ref}
      type={type}
      className={cn(
        'h-7 px-2.5 bg-surface text-foreground border border-border rounded-md text-sm outline-none',
        'focus:border-accent focus:ring-2 focus:ring-accent-soft focus:ring-offset-0',
        'disabled:opacity-50 disabled:cursor-not-allowed',
        'placeholder:text-foreground-subtle',
        className
      )}
      {...props}
    />
  )
);
Input.displayName = 'Input';
