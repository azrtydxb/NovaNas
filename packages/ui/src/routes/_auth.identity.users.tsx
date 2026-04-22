import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';
import { createFileRoute } from '@tanstack/react-router';
import { Plus, Users } from 'lucide-react';

export const Route = createFileRoute('/_auth/identity/users')({
  component: UsersPage,
});

function UsersPage() {
  return (
    <ShellScreen
      title='Users'
      subtitle='Accounts managed through Keycloak.'
      icon={<Users size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> New user
        </Button>
      }
      upcoming={[
        'User list with roles, last login and MFA state',
        'API tokens and SSH keys per user',
        'Federated identity linkage (AD/LDAP/OIDC)',
        'Quota and dataset ownership summary',
      ]}
    />
  );
}
