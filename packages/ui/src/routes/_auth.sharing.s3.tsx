import { createFileRoute } from '@tanstack/react-router';
import { Cloud } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';

export const Route = createFileRoute('/_auth/sharing/s3')({
  component: S3Page,
});

function S3Page() {
  return (
    <ShellScreen
      title='S3'
      subtitle='Native chunk-engine object storage (ObjectStore, Buckets, BucketUsers).'
      icon={<Cloud size={28} />}
      upcoming={[
        'ObjectStore endpoints and TLS',
        'Bucket browser with lifecycle rules',
        'BucketUsers with access/secret key rotation',
        'Usage and egress analytics',
      ]}
    />
  );
}
