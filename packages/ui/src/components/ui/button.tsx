import { cn } from '@/lib/cn';
import { Slot } from '@radix-ui/react-slot';
import { type VariantProps, cva } from 'class-variance-authority';
import { forwardRef } from 'react';

const buttonVariants = cva(
  'inline-flex items-center justify-center gap-1.5 rounded-md whitespace-nowrap text-sm transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent disabled:pointer-events-none disabled:opacity-50',
  {
    variants: {
      variant: {
        default:
          'bg-elevated text-foreground border border-border hover:bg-[var(--bg-4)] hover:text-foreground',
        primary:
          'bg-accent text-accent-fg font-medium hover:brightness-110 border border-transparent',
        ghost:
          'bg-transparent text-foreground-muted hover:bg-elevated hover:text-foreground border border-transparent',
        outline:
          'bg-transparent border border-border text-foreground-muted hover:bg-elevated hover:text-foreground',
        danger: 'bg-transparent text-danger border border-transparent hover:bg-danger-soft',
        link: 'bg-transparent text-accent underline-offset-2 hover:underline border-0 h-auto p-0',
      },
      size: {
        sm: 'h-6 px-2 text-xs',
        md: 'h-7 px-3 text-sm',
        lg: 'h-9 px-4 text-md',
        icon: 'h-7 w-7',
      },
    },
    defaultVariants: {
      variant: 'default',
      size: 'md',
    },
  }
);

export interface ButtonProps
  extends React.ButtonHTMLAttributes<HTMLButtonElement>,
    VariantProps<typeof buttonVariants> {
  asChild?: boolean;
}

export const Button = forwardRef<HTMLButtonElement, ButtonProps>(
  ({ className, variant, size, asChild, ...props }, ref) => {
    const Comp = asChild ? Slot : 'button';
    return (
      <Comp ref={ref} className={cn(buttonVariants({ variant, size }), className)} {...props} />
    );
  }
);
Button.displayName = 'Button';

export { buttonVariants };
