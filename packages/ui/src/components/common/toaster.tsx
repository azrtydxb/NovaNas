import { Toast, ToastDescription, ToastTitle } from '@/components/ui/toast';
import { useToastActions, useToasts } from '@/hooks/use-toast';

export function Toaster() {
  const toasts = useToasts();
  const { dismiss } = useToastActions();
  return (
    <>
      {toasts.map((t) => (
        <Toast
          key={t.id}
          tone={t.tone ?? 'default'}
          duration={t.duration ?? 4000}
          onOpenChange={(open) => {
            if (!open) dismiss(t.id);
          }}
          open
        >
          <div className='flex flex-col gap-0.5'>
            <ToastTitle>{t.title}</ToastTitle>
            {t.description && <ToastDescription>{t.description}</ToastDescription>}
          </div>
        </Toast>
      ))}
    </>
  );
}
