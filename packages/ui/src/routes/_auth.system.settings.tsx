import { createFileRoute } from '@tanstack/react-router';
import { Settings2 } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';

export const Route = createFileRoute('/_auth/system/settings')({
  component: SettingsPage,
});

function SettingsPage() {
  return (
    <ShellScreen
      title='Settings'
      subtitle='System-wide configuration.'
      icon={<Settings2 size={28} />}
      upcoming={[
        'Hostname, timezone and NTP',
        'Display preferences (theme, density, accent)',
        'Email / ntfy / Pushover alert routing',
        'Config backup schedule and target',
      ]}
    />
  );
}
