// TODO(i18n-wave-12): strings on this page are still raw English. Migrate to <Trans>/i18n._() once wave 12 is green.
import {
  useCreateKeycloakRealm,
  useDeleteKeycloakRealm,
  useKeycloakRealms,
} from '@/api/keycloak-realms';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { StatusDot } from '@/components/common/status-dot';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from '@/components/ui/dialog';
import { Input } from '@/components/ui/input';
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from '@/components/ui/select';
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
import type { KeycloakRealm } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { Plus, ShieldCheck, Trash2 } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/identity/providers')({
  component: ProvidersPage,
});

function ProvidersPage() {
  const { canMutate } = useAuth();
  const q = useKeycloakRealms();
  const del = useDeleteKeycloakRealm();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title='Identity providers'
        subtitle='Keycloak realms and upstream federation (AD / LDAP / OIDC).'
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> New realm
            </Button>
          ) : null
        }
      />
      {q.isLoading ? (
        <Skeleton className='h-24' />
      ) : q.isError ? (
        <EmptyState
          icon={<ShieldCheck size={28} />}
          title='Unable to load realms'
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>Retry</Button>}
        />
      ) : (q.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<ShieldCheck size={28} />}
          title='No realms configured'
          description='Connect an upstream AD / LDAP / OIDC provider via a Keycloak realm.'
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> New realm
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
                <TableHeaderCell>Display</TableHeaderCell>
                <TableHeaderCell>Federations</TableHeaderCell>
                <TableHeaderCell>Users</TableHeaderCell>
                <TableHeaderCell>Groups</TableHeaderCell>
                <TableHeaderCell>Last sync</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {q.data!.map((r: KeycloakRealm) => (
                <TableRow key={r.metadata.name}>
                  <TableCell>
                    <StatusDot
                      tone={r.status?.phase === 'Active' ? 'ok' : 'idle'}
                      className='mr-2'
                    />
                    <span className='mono text-xs'>{r.metadata.name}</span>
                  </TableCell>
                  <TableCell className='text-xs'>{r.spec.displayName ?? '—'}</TableCell>
                  <TableCell>
                    <div className='flex gap-1 flex-wrap'>
                      {(r.spec.federations ?? []).map((f, i) => (
                        <Badge key={i}>{f.type}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className='mono text-xs'>{r.status?.userCount ?? 0}</TableCell>
                  <TableCell className='mono text-xs'>{r.status?.groupCount ?? 0}</TableCell>
                  <TableCell className='mono text-xs'>{r.status?.lastSync ?? '—'}</TableCell>
                  <TableCell className='text-right'>
                    {mayMutate && (
                      <Button
                        size='sm'
                        variant='danger'
                        onClick={async () => {
                          try {
                            await del.mutateAsync(r.metadata.name);
                            toast.success('Realm deleted', r.metadata.name);
                          } catch (e) {
                            toast.error('Delete failed', (e as Error).message);
                          }
                        }}
                      >
                        <Trash2 size={12} />
                      </Button>
                    )}
                  </TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        </div>
      )}
      <CreateRealmDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateRealmDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateKeycloakRealm();
  const toast = useToast();
  const [name, setName] = useState('');
  const [displayName, setDisplayName] = useState('');
  const [enableFederation, setEnableFederation] = useState(false);
  const [fedType, setFedType] = useState<'activeDirectory' | 'ldap' | 'oidc'>('ldap');
  const [fedUrl, setFedUrl] = useState('');
  const [baseDn, setBaseDn] = useState('');

  const submit = async () => {
    if (!name) {
      toast.error('Missing name');
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          displayName: displayName || undefined,
          federations:
            enableFederation && fedUrl
              ? [
                  {
                    type: fedType,
                    connection: { url: fedUrl, baseDn: baseDn || undefined },
                  },
                ]
              : undefined,
        },
      });
      toast.success('Realm created', name);
      setName('');
      setDisplayName('');
      setFedUrl('');
      setBaseDn('');
      setEnableFederation(false);
      onOpenChange(false);
    } catch (e) {
      toast.error('Create failed', (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New Keycloak realm</DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label='Name' required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label='Display name'>
            <Input value={displayName} onChange={(e) => setDisplayName(e.target.value)} />
          </FormField>
          <div className='border border-border rounded-sm p-2 flex flex-col gap-2'>
            <label className='flex items-center gap-2 text-sm'>
              <input
                type='checkbox'
                checked={enableFederation}
                onChange={(e) => setEnableFederation(e.target.checked)}
              />
              Attach upstream federation
            </label>
            {enableFederation && (
              <div className='flex flex-col gap-2 pl-4'>
                <FormField label='Type'>
                  <Select value={fedType} onValueChange={(v) => setFedType(v as typeof fedType)}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='activeDirectory'>Active Directory</SelectItem>
                      <SelectItem value='ldap'>LDAP</SelectItem>
                      <SelectItem value='oidc'>OIDC</SelectItem>
                    </SelectContent>
                  </Select>
                </FormField>
                <FormField label='URL' required>
                  <Input
                    value={fedUrl}
                    onChange={(e) => setFedUrl(e.target.value)}
                    placeholder='ldaps://ad.corp:636'
                  />
                </FormField>
                <FormField label='Base DN'>
                  <Input value={baseDn} onChange={(e) => setBaseDn(e.target.value)} />
                </FormField>
              </div>
            )}
          </div>
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
