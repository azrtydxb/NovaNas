import * as ToastPrimitive from '@radix-ui/react-toast';
import { cva, type VariantProps } from 'class-variance-authority';
import { X } from 'lucide-react';
import { forwardRef } from 'react';
import { cn } from '@/lib/cn';

export const ToastProvider = ToastPrimitive.Provider;

export const ToastViewport = forwardRef<
  React.ElementRef<typeof ToastPrimitive.Viewport>,
  React.ComponentPropsWithoutRef<typeof ToastPrimitive.Viewport>
>(({ className, ...props }, ref) => (
  <ToastPrimitive.Viewport
    ref={ref}
    className={cn(
      'fixed bottom-4 right-4 z-[100] flex max-h-screen w-full max-w-sm flex-col gap-2 outline-none',
      className
    )}
    {...props}
  />
));
ToastViewport.displayName = 'ToastViewport';

const toastVariants = cva(
  'group pointer-events-auto relative flex w-full items-start justify-between gap-3 overflow-hidden rounded-md border border-border bg-panel p-3 pr-8 shadow-soft-md data-[state=open]:animate-in data-[state=closed]:animate-out',
  {
    variants: {
      tone: {
        default: '',
        success: 'border-success/30',
        warning: 'border-warning/30',
        danger: 'border-danger/30',
      },
    },
    defaultVariants: { tone: 'default' },
  }
);

export interface ToastProps
  extends React.ComponentPropsWithoutRef<typeof ToastPrimitive.Root>,
    VariantProps<typeof toastVariants> {}

export const Toast = forwardRef<React.ElementRef<typeof ToastPrimitive.Root>, ToastProps>(
  ({ className, tone, ...props }, ref) => (
    <ToastPrimitive.Root ref={ref} className={cn(toastVariants({ tone }), className)} {...props} />
  )
);
Toast.displayName = 'Toast';

export const ToastTitle = forwardRef<
  React.ElementRef<typeof ToastPrimitive.Title>,
  React.ComponentPropsWithoutRef<typeof ToastPrimitive.Title>
>(({ className, ...props }, ref) => (
  <ToastPrimitive.Title
    ref={ref}
    className={cn('text-sm font-medium text-foreground', className)}
    {...props}
  />
));
ToastTitle.displayName = 'ToastTitle';

export const ToastDescription = forwardRef<
  React.ElementRef<typeof ToastPrimitive.Description>,
  React.ComponentPropsWithoutRef<typeof ToastPrimitive.Description>
>(({ className, ...props }, ref) => (
  <ToastPrimitive.Description
    ref={ref}
    className={cn('text-xs text-foreground-muted', className)}
    {...props}
  />
));
ToastDescription.displayName = 'ToastDescription';

export const ToastClose = forwardRef<
  React.ElementRef<typeof ToastPrimitive.Close>,
  React.ComponentPropsWithoutRef<typeof ToastPrimitive.Close>
>(({ className, ...props }, ref) => (
  <ToastPrimitive.Close
    ref={ref}
    className={cn(
      'absolute right-2 top-2 rounded-sm p-0.5 text-foreground-subtle hover:text-foreground focus-visible:outline-none',
      className
    )}
    toast-close=''
    {...props}
  >
    <X size={14} />
  </ToastPrimitive.Close>
));
ToastClose.displayName = 'ToastClose';
