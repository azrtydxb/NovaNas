import { cn } from '@/lib/cn';
import * as LabelPrimitive from '@radix-ui/react-label';
import { forwardRef } from 'react';

export const Label = forwardRef<
  React.ElementRef<typeof LabelPrimitive.Root>,
  React.ComponentPropsWithoutRef<typeof LabelPrimitive.Root>
>(({ className, ...props }, ref) => (
  <LabelPrimitive.Root
    ref={ref}
    className={cn(
      'text-2xs uppercase tracking-wider text-foreground-subtle leading-none',
      className
    )}
    {...props}
  />
));
Label.displayName = 'Label';
