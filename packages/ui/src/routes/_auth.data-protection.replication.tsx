import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import { createFileRoute } from '@tanstack/react-router';
import { ArrowLeftRight, Plus } from 'lucide-react';

export const Route = createFileRoute('/_auth/data-protection/replication')({
  component: ReplicationPage,
});

function ReplicationPage() {
  return (
    <ShellScreen
      title={i18n._('Replication')}
      subtitle={i18n._('Peer-to-peer replication to another NovaNas or compatible target.')}
      icon={<ArrowLeftRight size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> <Trans id='New replication' />
        </Button>
      }
      upcoming={[
        i18n._('Source dataset, target endpoint and throttle'),
        i18n._('Live progress, lag and last successful run'),
        i18n._('Failover / failback controls'),
        i18n._('End-to-end encryption configuration'),
      ]}
    />
  );
}
