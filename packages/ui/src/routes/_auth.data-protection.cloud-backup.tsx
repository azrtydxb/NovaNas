import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';
import { createFileRoute } from '@tanstack/react-router';
import { CloudUpload, Plus } from 'lucide-react';

export const Route = createFileRoute('/_auth/data-protection/cloud-backup')({
  component: CloudBackupPage,
});

function CloudBackupPage() {
  return (
    <ShellScreen
      title='Cloud Backup'
      subtitle='Ship encrypted snapshots to S3-compatible destinations.'
      icon={<CloudUpload size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> New backup job
        </Button>
      }
      upcoming={[
        'Encrypted, deduplicated backups with immutable retention',
        'Per-job schedule, bandwidth cap and destination',
        'Restore browser and verification runs',
        'Provider credentials managed via OpenBao',
      ]}
    />
  );
}
