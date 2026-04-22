import { ShellScreen } from '@/components/common/shell-screen';
import { Button } from '@/components/ui/button';
import { createFileRoute } from '@tanstack/react-router';
import { Plus, UsersRound } from 'lucide-react';

export const Route = createFileRoute('/_auth/identity/groups')({
  component: GroupsPage,
});

function GroupsPage() {
  return (
    <ShellScreen
      title='Groups'
      subtitle='Collections of users with shared roles and ACLs.'
      icon={<UsersRound size={28} />}
      actions={
        <Button variant='primary'>
          <Plus size={13} /> New group
        </Button>
      }
      upcoming={[
        'Group list with member count and roles',
        'ACL editor for shares and datasets',
        'Nested group relationships',
        'Sync state with external directory',
      ]}
    />
  );
}
