import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import { createFileRoute } from '@tanstack/react-router';
import { CloudUpload, Plus } from 'lucide-react';

export const Route = createFileRoute('/_auth/data-protection/cloud-backup')({
  component: CloudBackupPage,
});

function CloudBackupPage() {
  return (
    <ShellScreen
      title={i18n._('Cloud Backup')}
      subtitle={i18n._('Ship encrypted snapshots to S3-compatible destinations.')}
      icon={<CloudUpload size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> <Trans id='New backup job' />
        </Button>
      }
      upcoming={[
        i18n._('Encrypted, deduplicated backups with immutable retention'),
        i18n._('Per-job schedule, bandwidth cap and destination'),
        i18n._('Restore browser and verification runs'),
        i18n._('Provider credentials managed via OpenBao'),
      ]}
    />
  );
}
