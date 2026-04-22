import { createFileRoute } from '@tanstack/react-router';
import { ShieldCheck } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';

export const Route = createFileRoute('/_auth/system/certificates')({
  component: CertificatesPage,
});

function CertificatesPage() {
  return (
    <ShellScreen
      title='Certificates'
      subtitle='TLS certificates for the UI, APIs and sharing protocols.'
      icon={<ShieldCheck size={28} />}
      upcoming={[
        'Issued / imported certificates',
        'ACME (Let’s Encrypt) automation',
        'Expiry alerts and renewal status',
        'Bind certificates to services',
      ]}
    />
  );
}
