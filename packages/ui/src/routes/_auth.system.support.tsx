import { ShellScreen } from '@/components/common/shell-screen';
import { createFileRoute } from '@tanstack/react-router';
import { LifeBuoy } from 'lucide-react';

export const Route = createFileRoute('/_auth/system/support')({
  component: SupportPage,
});

function SupportPage() {
  return (
    <ShellScreen
      title='Support'
      subtitle='Diagnostics, support bundle, shutdown / reboot.'
      icon={<LifeBuoy size={28} />}
      upcoming={[
        'Support bundle generator (logs, state, config)',
        'Community and commercial support links',
        'Shutdown / reboot with safe drain',
        'Remote support tunnel (operator-approved)',
      ]}
    />
  );
}
