import { createFileRoute } from '@tanstack/react-router';
import { Plus, Share2 } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';

export const Route = createFileRoute('/_auth/sharing/shares')({
  component: SharesPage,
});

function SharesPage() {
  return (
    <ShellScreen
      title='Shares'
      subtitle='SMB + NFS exports on top of datasets.'
      icon={<Share2 size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> New share
        </Button>
      }
      upcoming={[
        'SMB/NFS share list with live connection counts',
        'Per-share ACLs, allowed hosts, encryption',
        'Connection activity and throughput',
        'Create share wizard with dataset picker',
      ]}
    />
  );
}
