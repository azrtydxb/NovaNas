import { useGroups } from '@/api/groups';
import { useCreateUser, useDeleteUser, useResetUserPassword, useUsers } from '@/api/users';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { StatusDot } from '@/components/common/status-dot';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeaderCell,
  TableRow,
} from '@/components/ui/table';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import type { User } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { KeyRound, Plus, Trash2, Users } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/identity/users')({
  component: UsersPage,
});

function UsersPage() {
  const { canMutate } = useAuth();
  const q = useUsers();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<User | null>(null);
  const [resetTarget, setResetTarget] = useState<User | null>(null);
  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title='Users'
        subtitle='Accounts managed through Keycloak.'
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> New user
            </Button>
          ) : null
        }
      />

      {q.isLoading ? (
        <Skeleton className='h-24' />
      ) : q.isError ? (
        <EmptyState
          icon={<Users size={28} />}
          title='Unable to load users'
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>Retry</Button>}
        />
      ) : (q.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<Users size={28} />}
          title='No users yet'
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> New user
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>Username</TableHeaderCell>
                <TableHeaderCell>Email</TableHeaderCell>
                <TableHeaderCell>Groups</TableHeaderCell>
                <TableHeaderCell>Admin</TableHeaderCell>
                <TableHeaderCell>Realm</TableHeaderCell>
                <TableHeaderCell>Last login</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {q.data!.map((u) => (
                <TableRow key={u.metadata.name}>
                  <TableCell>
                    <StatusDot
                      tone={u.status?.phase === 'Active' ? 'ok' : 'idle'}
                      className='mr-2'
                    />
                    <span className='mono text-xs'>{u.spec.username}</span>
                  </TableCell>
                  <TableCell className='text-xs'>{u.spec.email ?? '—'}</TableCell>
                  <TableCell>
                    <div className='flex gap-1 flex-wrap'>
                      {(u.spec.groups ?? []).map((g) => (
                        <Badge key={g}>{g}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell>{u.spec.admin ? <Badge>admin</Badge> : null}</TableCell>
                  <TableCell className='mono text-xs'>{u.spec.realm ?? '—'}</TableCell>
                  <TableCell className='mono text-xs'>{u.status?.lastLogin ?? '—'}</TableCell>
                  <TableCell className='text-right'>
                    {mayMutate && (
                      <div className='flex gap-1 justify-end'>
                        <Button size='sm' variant='ghost' onClick={() => setResetTarget(u)}>
                          <KeyRound size={12} />
                        </Button>
                        <Button size='sm' variant='danger' onClick={() => setDeleteTarget(u)}>
                          <Trash2 size={12} />
                        </Button>
                      </div>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <CreateUserDialog open={createOpen} onOpenChange={setCreateOpen} />
      <DeleteUserDialog user={deleteTarget} onOpenChange={(o) => !o && setDeleteTarget(null)} />
      <ResetPasswordDialog user={resetTarget} onOpenChange={(o) => !o && setResetTarget(null)} />
    </>
  );
}

function CreateUserDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateUser();
  const groups = useGroups();
  const toast = useToast();
  const [username, setUsername] = useState('');
  const [email, setEmail] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [admin, setAdmin] = useState(false);
  const [selectedGroups, setSelectedGroups] = useState<string[]>([]);

  const toggleGroup = (g: string) =>
    setSelectedGroups((cur) => (cur.includes(g) ? cur.filter((x) => x !== g) : [...cur, g]));

  const submit = async () => {
    if (!username) {
      toast.error('Missing username');
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name: username },
        spec: {
          username,
          email: email || undefined,
          displayName: displayName || undefined,
          admin: admin || undefined,
          groups: selectedGroups.length ? selectedGroups : undefined,
        },
      });
      toast.success('User created', username);
      setUsername('');
      setEmail('');
      setDisplayName('');
      setAdmin(false);
      setSelectedGroups([]);
      onOpenChange(false);
    } catch (e) {
      toast.error('Create failed', (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New user</DialogTitle>
          <DialogDescription>Creates a user and reconciles into Keycloak.</DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label='Username' required>
            <Input value={username} onChange={(e) => setUsername(e.target.value)} />
          </FormField>
          <FormField label='Email'>
            <Input type='email' value={email} onChange={(e) => setEmail(e.target.value)} />
          </FormField>
          <FormField label='Display name'>
            <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
          </FormField>
          <div className='flex items-center gap-2 text-sm'>
            <Checkbox checked={admin} onCheckedChange={(v) => setAdmin(!!v)} />
            Admin role
          </div>
          <FormField label='Groups'>
            <div className='flex flex-col gap-1 border border-border rounded-sm p-2 max-h-32 overflow-y-auto'>
              {groups.isLoading ? (
                <span className='text-xs text-foreground-subtle'>Loading…</span>
              ) : (groups.data?.length ?? 0) === 0 ? (
                <span className='text-xs text-foreground-subtle'>No groups available.</span>
              ) : (
                groups.data!.map((g) => (
                  <div key={g.metadata.name} className='flex items-center gap-2 text-xs'>
                    <Checkbox
                      checked={selectedGroups.includes(g.metadata.name)}
                      onCheckedChange={() => toggleGroup(g.metadata.name)}
                    />
                    <span className='mono'>{g.metadata.name}</span>
                  </div>
                ))
              )}
            </div>
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? 'Creating…' : 'Create user'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteUserDialog({
  user,
  onOpenChange,
}: {
  user: User | null;
  onOpenChange: (v: boolean) => void;
}) {
  const del = useDeleteUser();
  const toast = useToast();
  return (
    <Dialog open={!!user} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete user?</DialogTitle>
          <DialogDescription>
            User <span className='mono'>{user?.spec.username}</span> will be removed.
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant='danger'
            disabled={del.isPending}
            onClick={async () => {
              if (!user) return;
              try {
                await del.mutateAsync(user.metadata.name);
                toast.success('Deleted', user.spec.username);
                onOpenChange(false);
              } catch (e) {
                toast.error('Delete failed', (e as Error).message);
              }
            }}
          >
            {del.isPending ? 'Deleting…' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ResetPasswordDialog({
  user,
  onOpenChange,
}: {
  user: User | null;
  onOpenChange: (v: boolean) => void;
}) {
  const reset = useResetUserPassword();
  const toast = useToast();
  const [password, setPassword] = useState('');
  return (
    <Dialog open={!!user} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Reset password</DialogTitle>
          <DialogDescription>
            Set a new password for <span className='mono'>{user?.spec.username}</span>.
          </DialogDescription>
        </DialogHeader>
        <FormField label='New password' required>
          <Input type='password' value={password} onChange={(e) => setPassword(e.target.value)} />
        </FormField>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant='primary'
            disabled={reset.isPending || !password}
            onClick={async () => {
              if (!user) return;
              try {
                await reset.mutateAsync({ name: user.metadata.name, password });
                toast.success('Password reset', user.spec.username);
                setPassword('');
                onOpenChange(false);
              } catch (e) {
                toast.error('Reset failed', (e as Error).message);
              }
            }}
          >
            {reset.isPending ? 'Saving…' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
