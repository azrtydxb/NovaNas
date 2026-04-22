import { useCreateGroup, useDeleteGroup, useGroups, useUpdateGroup } from '@/api/groups';
import { useUsers } from '@/api/users';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
import {
  Dialog,
  DialogContent,
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
import type { Group } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { Plus, Trash2, UsersRound } from 'lucide-react';
import { useEffect, useState } from 'react';

export const Route = createFileRoute('/_auth/identity/groups')({
  component: GroupsPage,
});

function GroupsPage() {
  const { canMutate } = useAuth();
  const q = useGroups();
  const [createOpen, setCreateOpen] = useState(false);
  const [editTarget, setEditTarget] = useState<Group | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Group | null>(null);
  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title='Groups'
        subtitle='Identity groups and membership.'
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> New group
            </Button>
          ) : null
        }
      />
      {q.isLoading ? (
        <Skeleton className='h-24' />
      ) : q.isError ? (
        <EmptyState
          icon={<UsersRound size={28} />}
          title='Unable to load groups'
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>Retry</Button>}
        />
      ) : (q.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<UsersRound size={28} />}
          title='No groups yet'
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> New group
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>Name</TableHeaderCell>
                <TableHeaderCell>Display name</TableHeaderCell>
                <TableHeaderCell>Members</TableHeaderCell>
                <TableHeaderCell>GID</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {q.data!.map((g) => (
                <TableRow key={g.metadata.name}>
                  <TableCell className='mono text-xs'>{g.metadata.name}</TableCell>
                  <TableCell className='text-xs'>{g.spec.displayName ?? '—'}</TableCell>
                  <TableCell className='text-xs'>
                    {g.status?.memberCount ?? g.spec.members?.length ?? 0}
                  </TableCell>
                  <TableCell className='mono text-xs'>{g.spec.gid ?? '—'}</TableCell>
                  <TableCell className='text-right'>
                    {mayMutate && (
                      <div className='flex gap-1 justify-end'>
                        <Button size='sm' variant='ghost' onClick={() => setEditTarget(g)}>
                          Edit
                        </Button>
                        <Button size='sm' variant='danger' onClick={() => setDeleteTarget(g)}>
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
      <CreateGroupDialog open={createOpen} onOpenChange={setCreateOpen} />
      <EditGroupDialog group={editTarget} onOpenChange={(o) => !o && setEditTarget(null)} />
      <DeleteGroupDialog group={deleteTarget} onOpenChange={(o) => !o && setDeleteTarget(null)} />
    </>
  );
}

function CreateGroupDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateGroup();
  const toast = useToast();
  const [name, setName] = useState('');
  const [displayName, setDisplayName] = useState('');

  const submit = async () => {
    if (!name) {
      toast.error('Missing name');
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: { name, displayName: displayName || undefined },
      });
      toast.success('Group created', name);
      setName('');
      setDisplayName('');
      onOpenChange(false);
    } catch (e) {
      toast.error('Create failed', (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New group</DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label='Name' required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label='Display name'>
            <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? 'Creating…' : 'Create'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function EditGroupDialog({
  group,
  onOpenChange,
}: {
  group: Group | null;
  onOpenChange: (v: boolean) => void;
}) {
  const users = useUsers();
  const update = useUpdateGroup(group?.metadata.name ?? '');
  const toast = useToast();
  const [members, setMembers] = useState<string[]>([]);

  useEffect(() => {
    if (group) setMembers(group.spec.members ?? []);
  }, [group]);

  const toggle = (u: string) =>
    setMembers((cur) => (cur.includes(u) ? cur.filter((x) => x !== u) : [...cur, u]));

  const submit = async () => {
    if (!group) return;
    try {
      await update.mutateAsync({ spec: { members } });
      toast.success('Group updated', group.metadata.name);
      onOpenChange(false);
    } catch (e) {
      toast.error('Update failed', (e as Error).message);
    }
  };

  return (
    <Dialog open={!!group} onOpenChange={(v) => !v && onOpenChange(false)}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Edit members — {group?.metadata.name}</DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-1 border border-border rounded-sm p-2 max-h-64 overflow-y-auto'>
          {users.isLoading ? (
            <span className='text-xs text-foreground-subtle'>Loading users…</span>
          ) : (users.data?.length ?? 0) === 0 ? (
            <span className='text-xs text-foreground-subtle'>No users available.</span>
          ) : (
            users.data!.map((u) => (
              <div key={u.metadata.name} className='flex items-center gap-2 text-xs'>
                <Checkbox
                  checked={members.includes(u.metadata.name)}
                  onCheckedChange={() => toggle(u.metadata.name)}
                />
                <span className='mono'>{u.spec.username}</span>
                {u.spec.email && <span className='text-foreground-subtle'>{u.spec.email}</span>}
              </div>
            ))
          )}
        </div>
        <div className='text-xs text-foreground-subtle'>
          <Badge>{members.length}</Badge> member{members.length === 1 ? '' : 's'}
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant='primary' onClick={submit} disabled={update.isPending}>
            {update.isPending ? 'Saving…' : 'Save'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteGroupDialog({
  group,
  onOpenChange,
}: {
  group: Group | null;
  onOpenChange: (v: boolean) => void;
}) {
  const del = useDeleteGroup();
  const toast = useToast();
  return (
    <Dialog open={!!group} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete group?</DialogTitle>
        </DialogHeader>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant='danger'
            disabled={del.isPending}
            onClick={async () => {
              if (!group) return;
              try {
                await del.mutateAsync(group.metadata.name);
                toast.success('Deleted', group.metadata.name);
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
