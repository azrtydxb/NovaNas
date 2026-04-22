import { createFileRoute } from '@tanstack/react-router';
import { FolderTree, Plus } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';

export const Route = createFileRoute('/_auth/storage/datasets')({
  component: DatasetsPage,
});

function DatasetsPage() {
  return (
    <ShellScreen
      title='Datasets'
      subtitle='Logical buckets for user data. SMB, NFS, block and S3.'
      icon={<FolderTree size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> New dataset
        </Button>
      }
      upcoming={[
        'Tree view of datasets and children',
        'Quota, protection policy, encryption state',
        'Snapshot count and last activity',
        'Create/edit dataset wizard with protocol exposure',
      ]}
    />
  );
}
