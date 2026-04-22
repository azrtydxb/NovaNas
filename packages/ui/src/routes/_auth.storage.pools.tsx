import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';
import { createFileRoute } from '@tanstack/react-router';
import { Database, Plus } from 'lucide-react';

export const Route = createFileRoute('/_auth/storage/pools')({
  component: PoolsPage,
});

function PoolsPage() {
  return (
    <ShellScreen
      title='Pools'
      subtitle='Hot / warm / cold storage tiers backed by the chunk engine.'
      icon={<Database size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> Create pool
        </Button>
      }
      upcoming={[
        'Pool list with tier, protection, capacity and throughput',
        'Per-pool detail: devices, policies, scrub/rebuild state',
        'Create/expand pool wizard',
        'Policy editor (rep × N, EC k+m, placement constraints)',
      ]}
    />
  );
}
