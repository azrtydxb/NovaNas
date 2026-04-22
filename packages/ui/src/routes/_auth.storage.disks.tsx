import { createFileRoute } from '@tanstack/react-router';
import { HardDrive } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';

export const Route = createFileRoute('/_auth/storage/disks')({
  component: DisksPage,
});

function DisksPage() {
  return (
    <ShellScreen
      title='Disks'
      subtitle='Physical devices, enclosures and slots.'
      icon={<HardDrive size={28} />}
      upcoming={[
        'Enclosure map with slot-accurate visualization',
        'Per-disk SMART, temperature, hours and wear',
        'Role (data / spare / cache) and pool membership',
        'Drain, identify (LED), retire and replace actions',
      ]}
    />
  );
}
