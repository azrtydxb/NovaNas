// TODO(i18n-wave-12): strings on this page are still raw English. Migrate to <Trans>/i18n._() once wave 12 is green.
import {
  type ApiTokenCreateResponse,
  useApiTokens,
  useCreateApiToken,
  useRevokeApiToken,
} from '@/api/api-tokens';
import { useCreateSshKey, useDeleteSshKey, useSshKeys } from '@/api/ssh-keys';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { createFileRoute } from '@tanstack/react-router';
import { KeyRound, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/identity/tokens')({
  component: TokensPage,
});

function TokensPage() {
  return (
    <>
      <PageHeader title='Tokens & keys' subtitle='API tokens and SSH public keys.' />
      <Tabs defaultValue='api'>
        <TabsList>
          <TabsTrigger value='api'>API tokens</TabsTrigger>
          <TabsTrigger value='ssh'>SSH keys</TabsTrigger>
        </TabsList>
        <TabsContent value='api'>
          <ApiTokensTab />
        </TabsContent>
        <TabsContent value='ssh'>
          <SshKeysTab />
        </TabsContent>
      </Tabs>
    </>
  );
}

function ApiTokensTab() {
  const { canMutate } = useAuth();
  const q = useApiTokens();
  const revoke = useRevokeApiToken();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const [issued, setIssued] = useState<ApiTokenCreateResponse | null>(null);
  const mayMutate = canMutate();

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex justify-end'>
        {mayMutate && (
          <Button variant='primary' onClick={() => setCreateOpen(true)}>
            <Plus size={13} /> New token
          </Button>
        )}
      </div>
      {q.isLoading ? (
        <Skeleton className='h-24' />
      ) : q.isError ? (
        <EmptyState
          icon={<KeyRound size={28} />}
          title='Unable to load tokens'
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>Retry</Button>}
        />
      ) : (q.data?.length ?? 0) === 0 ? (
        <EmptyState icon={<KeyRound size={28} />} title='No API tokens yet' />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>Name</TableHeaderCell>
                <TableHeaderCell>Owner</TableHeaderCell>
                <TableHeaderCell>Scopes</TableHeaderCell>
                <TableHeaderCell>Expires</TableHeaderCell>
                <TableHeaderCell>Last used</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {q.data!.map((t) => (
                <TableRow key={t.metadata.name}>
                  <TableCell className='mono text-xs'>{t.metadata.name}</TableCell>
                  <TableCell className='mono text-xs'>{t.spec.owner}</TableCell>
                  <TableCell>
                    <div className='flex gap-1 flex-wrap'>
                      {t.spec.scopes.map((s) => (
                        <Badge key={s}>{s}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className='mono text-xs'>{t.spec.expiresAt ?? 'never'}</TableCell>
                  <TableCell className='mono text-xs'>{t.status?.lastUsedAt ?? '—'}</TableCell>
                  <TableCell className='text-right'>
                    {mayMutate && (
                      <Button
                        size='sm'
                        variant='danger'
                        onClick={async () => {
                          try {
                            await revoke.mutateAsync(t.metadata.name);
                            toast.success('Token revoked', t.metadata.name);
                          } catch (e) {
                            toast.error('Revoke failed', (e as Error).message);
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
      <CreateApiTokenDialog open={createOpen} onOpenChange={setCreateOpen} onIssued={setIssued} />
      <IssuedTokenDialog issued={issued} onClose={() => setIssued(null)} />
    </div>
  );
}

function CreateApiTokenDialog({
  open,
  onOpenChange,
  onIssued,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
  onIssued: (r: ApiTokenCreateResponse) => void;
}) {
  const create = useCreateApiToken();
  const toast = useToast();
  const [name, setName] = useState('');
  const [owner, setOwner] = useState('');
  const [scopes, setScopes] = useState('read,write');
  const [expiresAt, setExpiresAt] = useState('');

  const submit = async () => {
    if (!name || !owner) {
      toast.error('Missing fields', 'Name and owner required.');
      return;
    }
    try {
      const res = await create.mutateAsync({
        metadata: { name },
        spec: {
          owner,
          scopes: scopes
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean),
          expiresAt: expiresAt || undefined,
        },
      });
      toast.success('Token created', name);
      onIssued(res);
      setName('');
      setOwner('');
      setScopes('read,write');
      setExpiresAt('');
      onOpenChange(false);
    } catch (e) {
      toast.error('Create failed', (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New API token</DialogTitle>
          <DialogDescription>
            The token secret is displayed once after creation. Copy it now.
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label='Name' required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label='Owner' required>
            <Input value={owner} onChange={(e) => setOwner(e.target.value)} />
          </FormField>
          <FormField label='Scopes' hint='Comma-separated'>
            <Input value={scopes} onChange={(e) => setScopes(e.target.value)} />
          </FormField>
          <FormField label='Expires at (ISO 8601)'>
            <Input
              value={expiresAt}
              onChange={(e) => setExpiresAt(e.target.value)}
              placeholder='2026-12-31T00:00:00Z'
            />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? 'Creating…' : 'Create token'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function IssuedTokenDialog({
  issued,
  onClose,
}: {
  issued: ApiTokenCreateResponse | null;
  onClose: () => void;
}) {
  const toast = useToast();
  return (
    <Dialog open={!!issued} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Save your token</DialogTitle>
          <DialogDescription>
            This secret is shown only once. Store it somewhere safe.
          </DialogDescription>
        </DialogHeader>
        <div className='mono text-xs p-3 bg-elevated rounded-sm break-all select-all'>
          {issued?.secret}
        </div>
        <DialogFooter>
          <Button
            variant='ghost'
            onClick={() => {
              if (issued?.secret) {
                navigator.clipboard
                  ?.writeText(issued.secret)
                  .then(() => toast.success('Copied'))
                  .catch(() => toast.error('Copy failed'));
              }
            }}
          >
            Copy
          </Button>
          <Button variant='primary' onClick={onClose}>
            I saved it
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function SshKeysTab() {
  const { canMutate } = useAuth();
  const q = useSshKeys();
  const del = useDeleteSshKey();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex justify-end'>
        {mayMutate && (
          <Button variant='primary' onClick={() => setCreateOpen(true)}>
            <Plus size={13} /> Add SSH key
          </Button>
        )}
      </div>
      {q.isLoading ? (
        <Skeleton className='h-24' />
      ) : q.isError ? (
        <EmptyState
          icon={<KeyRound size={28} />}
          title='Unable to load SSH keys'
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>Retry</Button>}
        />
      ) : (q.data?.length ?? 0) === 0 ? (
        <EmptyState icon={<KeyRound size={28} />} title='No SSH keys yet' />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>Name</TableHeaderCell>
                <TableHeaderCell>Owner</TableHeaderCell>
                <TableHeaderCell>Type</TableHeaderCell>
                <TableHeaderCell>Fingerprint</TableHeaderCell>
                <TableHeaderCell>Comment</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {q.data!.map((k) => (
                <TableRow key={k.metadata.name}>
                  <TableCell className='mono text-xs'>{k.metadata.name}</TableCell>
                  <TableCell className='mono text-xs'>{k.spec.owner}</TableCell>
                  <TableCell className='text-xs'>{k.status?.keyType ?? '—'}</TableCell>
                  <TableCell className='mono text-xs truncate max-w-xs'>
                    {k.status?.fingerprint ?? '—'}
                  </TableCell>
                  <TableCell className='text-xs'>{k.spec.comment ?? '—'}</TableCell>
                  <TableCell className='text-right'>
                    {mayMutate && (
                      <Button
                        size='sm'
                        variant='danger'
                        onClick={async () => {
                          try {
                            await del.mutateAsync(k.metadata.name);
                            toast.success('Key deleted', k.metadata.name);
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
      <CreateSshKeyDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}

function CreateSshKeyDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateSshKey();
  const toast = useToast();
  const [name, setName] = useState('');
  const [owner, setOwner] = useState('');
  const [publicKey, setPublicKey] = useState('');
  const [comment, setComment] = useState('');

  const submit = async () => {
    if (!name || !owner || !publicKey) {
      toast.error('Missing fields');
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: { owner, publicKey: publicKey.trim(), comment: comment || undefined },
      });
      toast.success('SSH key added', name);
      setName('');
      setOwner('');
      setPublicKey('');
      setComment('');
      onOpenChange(false);
    } catch (e) {
      toast.error('Create failed', (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Add SSH key</DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label='Name' required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label='Owner' required>
            <Input value={owner} onChange={(e) => setOwner(e.target.value)} />
          </FormField>
          <FormField label='Public key' required>
            <textarea
              value={publicKey}
              onChange={(e) => setPublicKey(e.target.value)}
              rows={4}
              className='w-full rounded-sm border border-border bg-surface p-2 text-xs mono'
              placeholder='ssh-ed25519 AAAA...'
            />
          </FormField>
          <FormField label='Comment'>
            <Input value={comment} onChange={(e) => setComment(e.target.value)} />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? 'Adding…' : 'Add key'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
