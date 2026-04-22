import { createFileRoute } from '@tanstack/react-router';
import { Camera, Plus } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';

export const Route = createFileRoute('/_auth/storage/snapshots')({
  component: SnapshotsPage,
});

function SnapshotsPage() {
  return (
    <ShellScreen
      title='Snapshots'
      subtitle='Immutable, chunk-deduplicated. Retention via SnapshotSchedule.'
      icon={<Camera size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> New snapshot
        </Button>
      }
      upcoming={[
        'Snapshot browser with filters by dataset and label',
        'Clone, rollback, restore-to-new-dataset actions',
        'Schedule linkage (auto / pre-update / manual)',
        'Inline diff and retention visualization',
      ]}
    />
  );
}
