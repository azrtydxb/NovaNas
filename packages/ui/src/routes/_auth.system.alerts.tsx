import { ShellScreen } from '@/components/common/shell-screen';
import { createFileRoute } from '@tanstack/react-router';
import { BellRing } from 'lucide-react';

export const Route = createFileRoute('/_auth/system/alerts')({
  component: AlertsPage,
});

function AlertsPage() {
  return (
    <ShellScreen
      title='Alerts'
      subtitle='Active and historical alerts across the system.'
      icon={<BellRing size={28} />}
      upcoming={[
        'Active / firing alerts with severity',
        'Acknowledge and silence actions',
        'AlertChannel routing to email, ntfy, Pushover, browser push',
        'History with MTTR analytics',
      ]}
    />
  );
}
