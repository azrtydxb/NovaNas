import { createFileRoute } from '@tanstack/react-router';
import { DownloadCloud } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';

export const Route = createFileRoute('/_auth/system/updates')({
  component: UpdatesPage,
});

function UpdatesPage() {
  return (
    <ShellScreen
      title='Updates'
      subtitle='OS images and app channel updates.'
      icon={<DownloadCloud size={28} />}
      upcoming={[
        'Current OS image and channel',
        'Available updates with release notes',
        'Staged-install flow with rollback point',
        'Auto-update schedule and maintenance windows',
      ]}
    />
  );
}
