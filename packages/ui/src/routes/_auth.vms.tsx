import { createFileRoute } from '@tanstack/react-router';
import { Plus, Server } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';

export const Route = createFileRoute('/_auth/vms')({
  component: VmsPage,
});

function VmsPage() {
  return (
    <ShellScreen
      title='Virtual Machines'
      subtitle='KubeVirt-backed VMs with SPICE console, ISO library and GPU passthrough.'
      icon={<Server size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> New VM
        </Button>
      }
      upcoming={[
        'VM grid with thumbnail, state and resource usage',
        'SPICE console (delivered in a later wave)',
        'ISO library and GPU device picker',
        'Snapshot, clone and migrate actions',
      ]}
    />
  );
}
