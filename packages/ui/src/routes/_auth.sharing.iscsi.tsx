import { ShellScreen } from '@/components/common/shell-screen';
import { createFileRoute } from '@tanstack/react-router';
import { HardDrive } from 'lucide-react';

export const Route = createFileRoute('/_auth/sharing/iscsi')({
  component: IscsiPage,
});

function IscsiPage() {
  return (
    <ShellScreen
      title='iSCSI / NVMe-oF'
      subtitle='Block targets for VM disks and bare-metal iSCSI initiators.'
      icon={<HardDrive size={28} />}
      upcoming={[
        'Targets, portals and CHAP auth configuration',
        'LUN-to-dataset mapping',
        'Initiator access control and active sessions',
        'NVMe-oF (TCP) subsystem management',
      ]}
    />
  );
}
