import { createFileRoute } from '@tanstack/react-router';
import { AppWindow, Plus } from 'lucide-react';
import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';

export const Route = createFileRoute('/_auth/apps')({
  component: AppsPage,
});

function AppsPage() {
  return (
    <ShellScreen
      title='Apps'
      subtitle='Curated catalog + installed apps running on k3s.'
      icon={<AppWindow size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> Install app
        </Button>
      }
      upcoming={[
        'Catalog grid with categories and tags',
        'Installed apps with health, endpoints and storage',
        'One-click install wizard with sane defaults',
        'Update notifications and rollback',
      ]}
    />
  );
}
