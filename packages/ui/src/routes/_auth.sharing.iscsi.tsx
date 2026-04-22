import { ShellScreen } from '@/components/common/shell-screen';
import { i18n } from '@/lib/i18n';
import { createFileRoute } from '@tanstack/react-router';
import { HardDrive } from 'lucide-react';

export const Route = createFileRoute('/_auth/sharing/iscsi')({
  component: IscsiPage,
});

function IscsiPage() {
  return (
    <ShellScreen
      title={i18n._('iSCSI / NVMe-oF')}
      subtitle={i18n._('Block targets for VM disks and bare-metal iSCSI initiators.')}
      icon={<HardDrive size={28} />}
      upcoming={[
        i18n._('Targets, portals and CHAP auth configuration'),
        i18n._('LUN-to-dataset mapping'),
        i18n._('Initiator access control and active sessions'),
        i18n._('NVMe-oF (TCP) subsystem management'),
      ]}
    />
  );
}
