import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';
import { createFileRoute } from '@tanstack/react-router';
import { ArrowLeftRight, Plus } from 'lucide-react';

export const Route = createFileRoute('/_auth/data-protection/replication')({
  component: ReplicationPage,
});

function ReplicationPage() {
  return (
    <ShellScreen
      title='Replication'
      subtitle='Peer-to-peer replication to another NovaNas or compatible target.'
      icon={<ArrowLeftRight size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> New replication
        </Button>
      }
      upcoming={[
        'Source dataset, target endpoint and throttle',
        'Live progress, lag and last successful run',
        'Failover / failback controls',
        'End-to-end encryption configuration',
      ]}
    />
  );
}
