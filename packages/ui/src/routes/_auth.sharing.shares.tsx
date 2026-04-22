import { useDatasets } from '@/api/datasets';
import { useNfsServers } from '@/api/nfs-servers';
import { type ShareCreateBody, useCreateShare, useDeleteShare, useShares } from '@/api/shares';
import { useSmbServers } from '@/api/smb-servers';
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
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import type { Share, ShareAccessEntry, ShareSpec } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { Plus, Share2, Trash2, X } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/sharing/shares')({
  component: SharesPage,
});

function SharesPage() {
  const { canMutate } = useAuth();
  const shares = useShares();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Share | null>(null);
  const [selected, setSelected] = useState<Share | null>(null);
  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title={i18n._('Shares')}
        subtitle={i18n._('SMB + NFS exports on top of datasets.')}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New share' />
            </Button>
          ) : null
        }
      />

      {shares.isLoading ? (
        <div className='flex flex-col gap-2'>
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className='h-9' />
          ))}
        </div>
      ) : shares.isError ? (
        <EmptyState
          icon={<Share2 size={28} />}
          title={i18n._('Unable to load shares')}
          description={(shares.error as Error)?.message ?? i18n._('Try again in a moment.')}
          action={<Button onClick={() => shares.refetch()}>{i18n._('Retry')}</Button>}
        />
      ) : (shares.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<Share2 size={28} />}
          title={i18n._('No shares yet')}
          description={i18n._('Expose a dataset over SMB, NFS, or both.')}
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> <Trans id='New share' />
              </Button>
            ) : undefined
          }
        />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>
                  <Trans id='Name' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Dataset' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Path' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Protocols' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Read only' />
                </TableHeaderCell>
                <TableHeaderCell className='text-right'>
                  <Trans id='Actions' />
                </TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {shares.data!.map((sh) => {
                const phase = sh.status?.phase ?? 'Pending';
                const tone = phase === 'Active' ? 'ok' : phase === 'Failed' ? 'err' : 'idle';
                const readOnly =
                  !!sh.spec.protocols.smb?.readOnly && !!sh.spec.protocols.nfs?.readOnly;
                return (
                  <TableRow
                    key={sh.metadata.name}
                    className='cursor-pointer'
                    onClick={() => setSelected(sh)}
                  >
                    <TableCell>
                      <StatusDot tone={tone} className='mr-2' />
                      <span className='text-foreground font-medium'>{sh.metadata.name}</span>
                    </TableCell>
                    <TableCell className='mono text-xs'>{sh.spec.dataset}</TableCell>
                    <TableCell className='mono text-xs'>{sh.spec.path}</TableCell>
                    <TableCell>
                      <div className='flex gap-1'>
                        {sh.spec.protocols.smb && <Badge>SMB</Badge>}
                        {sh.spec.protocols.nfs && <Badge>NFS</Badge>}
                      </div>
                    </TableCell>
                    <TableCell>
                      {readOnly ? (
                        <Badge>ro</Badge>
                      ) : (
                        <span className='text-foreground-subtle text-xs'>—</span>
                      )}
                    </TableCell>
                    <TableCell className='text-right'>
                      {mayMutate && (
                        <Button
                          size='sm'
                          variant='danger'
                          onClick={(e) => {
                            e.stopPropagation();
                            setDeleteTarget(sh);
                          }}
                        >
                          <Trash2 size={12} />
                        </Button>
                      )}
                    </TableCell>
                  </TableRow>
                );
              })}
            </TableBody>
          </Table>
        </div>
      )}

      <CreateShareDialog open={createOpen} onOpenChange={setCreateOpen} />
      <DeleteShareDialog share={deleteTarget} onOpenChange={(o) => !o && setDeleteTarget(null)} />
      <ShareDetailDrawer share={selected} onClose={() => setSelected(null)} />
    </>
  );
}

// -----------------------------------------------------------------------------
interface ShareForm {
  name: string;
  dataset: string;
  path: string;
  smbEnabled: boolean;
  smbServer: string;
  smbCaseSensitive: boolean;
  smbShadowCopies: boolean;
  nfsEnabled: boolean;
  nfsServer: string;
  nfsSquash: 'noRootSquash' | 'rootSquash' | 'allSquash';
  nfsAllowedNetworks: string;
  access: ShareAccessEntry[];
}

const defaultShareForm: ShareForm = {
  name: '',
  dataset: '',
  path: '/',
  smbEnabled: true,
  smbServer: '',
  smbCaseSensitive: false,
  smbShadowCopies: false,
  nfsEnabled: false,
  nfsServer: '',
  nfsSquash: 'rootSquash',
  nfsAllowedNetworks: '',
  access: [],
};

function CreateShareDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateShare();
  const datasets = useDatasets();
  const smbServers = useSmbServers();
  const nfsServers = useNfsServers();
  const toast = useToast();
  const [form, setForm] = useState<ShareForm>(defaultShareForm);
  const [newPrincipal, setNewPrincipal] = useState('');
  const [newMode, setNewMode] = useState<'rw' | 'ro' | 'none'>('rw');

  const reset = () => setForm(defaultShareForm);

  const submit = async () => {
    if (!form.name || !form.dataset || !form.path) {
      toast.error(i18n._('Missing fields'), i18n._('Name, dataset, and path are required.'));
      return;
    }
    if (!form.smbEnabled && !form.nfsEnabled) {
      toast.error(i18n._('Select a protocol'), i18n._('Enable SMB or NFS.'));
      return;
    }
    const spec: ShareSpec = {
      dataset: form.dataset,
      path: form.path,
      protocols: {},
      access: form.access.length ? form.access : undefined,
    };
    if (form.smbEnabled) {
      spec.protocols.smb = {
        server: form.smbServer,
        caseSensitive: form.smbCaseSensitive || undefined,
        shadowCopies: form.smbShadowCopies || undefined,
      };
    }
    if (form.nfsEnabled) {
      spec.protocols.nfs = {
        server: form.nfsServer,
        squash: form.nfsSquash,
        allowedNetworks: form.nfsAllowedNetworks
          ? form.nfsAllowedNetworks
              .split(',')
              .map((s) => s.trim())
              .filter(Boolean)
          : undefined,
      };
    }
    const body: ShareCreateBody = { metadata: { name: form.name }, spec };
    try {
      await create.mutateAsync(body);
      toast.success(i18n._('Share created'), form.name);
      reset();
      onOpenChange(false);
    } catch (err) {
      toast.error(i18n._('Failed to create share'), (err as Error)?.message);
    }
  };

  const addAccess = () => {
    const p = newPrincipal.trim();
    if (!p) return;
    const [kind, rest] = p.split(':', 2);
    const entry: ShareAccessEntry =
      kind === 'group'
        ? { principal: { group: rest ?? p }, mode: newMode }
        : { principal: { user: kind === 'user' ? (rest ?? p) : p }, mode: newMode };
    setForm({ ...form, access: [...form.access, entry] });
    setNewPrincipal('');
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-lg'>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New share' />
          </DialogTitle>
          <DialogDescription>
            <Trans id='Expose a dataset over SMB and/or NFS.' />
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input
              value={form.name}
              onChange={(e) => setForm({ ...form, name: e.target.value })}
              placeholder='media'
            />
          </FormField>
          <div className='grid grid-cols-2 gap-3'>
            <FormField label={i18n._('Dataset')} required>
              <Select value={form.dataset} onValueChange={(v) => setForm({ ...form, dataset: v })}>
                <SelectTrigger>
                  <SelectValue
                    placeholder={datasets.isLoading ? i18n._('Loading…') : i18n._('Select')}
                  />
                </SelectTrigger>
                <SelectContent>
                  {datasets.data?.map((d) => (
                    <SelectItem key={d.metadata.name} value={d.metadata.name}>
                      {d.metadata.name}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
            <FormField label={i18n._('Path')} required>
              <Input
                value={form.path}
                onChange={(e) => setForm({ ...form, path: e.target.value })}
                placeholder='/'
              />
            </FormField>
          </div>

          <div className='border border-border rounded-md p-2 flex flex-col gap-2'>
            <div className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={form.smbEnabled}
                onCheckedChange={(v) => setForm({ ...form, smbEnabled: !!v })}
              />
              SMB
            </div>
            {form.smbEnabled && (
              <div className='flex flex-col gap-2 pl-6'>
                <FormField label={i18n._('SMB server')}>
                  <Select
                    value={form.smbServer}
                    onValueChange={(v) => setForm({ ...form, smbServer: v })}
                  >
                    <SelectTrigger>
                      <SelectValue
                        placeholder={smbServers.isLoading ? i18n._('Loading…') : i18n._('Select')}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {smbServers.data?.map((s) => (
                        <SelectItem key={s.metadata.name} value={s.metadata.name}>
                          {s.metadata.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FormField>
                <div className='flex items-center gap-2 text-xs'>
                  <Checkbox
                    checked={form.smbCaseSensitive}
                    onCheckedChange={(v) => setForm({ ...form, smbCaseSensitive: !!v })}
                  />
                  <Trans id='Case sensitive' />
                </div>
                <div className='flex items-center gap-2 text-xs'>
                  <Checkbox
                    checked={form.smbShadowCopies}
                    onCheckedChange={(v) => setForm({ ...form, smbShadowCopies: !!v })}
                  />
                  <Trans id='Shadow copies (snapshots visible via VSS)' />
                </div>
              </div>
            )}
          </div>

          <div className='border border-border rounded-md p-2 flex flex-col gap-2'>
            <div className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={form.nfsEnabled}
                onCheckedChange={(v) => setForm({ ...form, nfsEnabled: !!v })}
              />
              NFS
            </div>
            {form.nfsEnabled && (
              <div className='flex flex-col gap-2 pl-6'>
                <FormField label={i18n._('NFS server')}>
                  <Select
                    value={form.nfsServer}
                    onValueChange={(v) => setForm({ ...form, nfsServer: v })}
                  >
                    <SelectTrigger>
                      <SelectValue
                        placeholder={nfsServers.isLoading ? i18n._('Loading…') : i18n._('Select')}
                      />
                    </SelectTrigger>
                    <SelectContent>
                      {nfsServers.data?.map((s) => (
                        <SelectItem key={s.metadata.name} value={s.metadata.name}>
                          {s.metadata.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </FormField>
                <FormField label={i18n._('Squash')}>
                  <Select
                    value={form.nfsSquash}
                    onValueChange={(v) =>
                      setForm({
                        ...form,
                        nfsSquash: v as ShareForm['nfsSquash'],
                      })
                    }
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value='rootSquash'>rootSquash</SelectItem>
                      <SelectItem value='noRootSquash'>noRootSquash</SelectItem>
                      <SelectItem value='allSquash'>allSquash</SelectItem>
                    </SelectContent>
                  </Select>
                </FormField>
                <FormField
                  label={i18n._('Allowed networks')}
                  hint={i18n._('Comma-separated CIDRs')}
                >
                  <Input
                    value={form.nfsAllowedNetworks}
                    onChange={(e) => setForm({ ...form, nfsAllowedNetworks: e.target.value })}
                    placeholder='10.0.0.0/8, 192.168.1.0/24'
                  />
                </FormField>
              </div>
            )}
          </div>

          <div className='border border-border rounded-md p-2 flex flex-col gap-2'>
            <div className='text-xs uppercase tracking-wider text-foreground-subtle'>
              <Trans id='Access rules' />
            </div>
            <ul className='flex flex-col gap-1'>
              {form.access.length === 0 && (
                <li className='text-xs text-foreground-subtle'>
                  <Trans id='No rules — inherits dataset ACL.' />
                </li>
              )}
              {form.access.map((a, i) => (
                <li
                  key={i}
                  className='flex items-center justify-between text-xs mono border border-border rounded-sm p-1'
                >
                  <span>
                    {a.principal.user ? `user:${a.principal.user}` : `group:${a.principal.group}`}{' '}
                    <Badge>{a.mode}</Badge>
                  </span>
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() =>
                      setForm({
                        ...form,
                        access: form.access.filter((_, j) => j !== i),
                      })
                    }
                  >
                    <X size={11} />
                  </Button>
                </li>
              ))}
            </ul>
            <div className='flex gap-2'>
              <Input
                placeholder={i18n._('user:alice or group:staff')}
                value={newPrincipal}
                onChange={(e) => setNewPrincipal(e.target.value)}
              />
              <Select value={newMode} onValueChange={(v) => setNewMode(v as typeof newMode)}>
                <SelectTrigger className='w-24'>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='rw'>rw</SelectItem>
                  <SelectItem value='ro'>ro</SelectItem>
                  <SelectItem value='none'>none</SelectItem>
                </SelectContent>
              </Select>
              <Button type='button' variant='ghost' onClick={addAccess}>
                <Trans id='Add' />
              </Button>
            </div>
          </div>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' disabled={create.isPending} onClick={submit}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create share' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function DeleteShareDialog({
  share,
  onOpenChange,
}: {
  share: Share | null;
  onOpenChange: (v: boolean) => void;
}) {
  const del = useDeleteShare();
  const toast = useToast();
  return (
    <Dialog open={!!share} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='Delete share?' />
          </DialogTitle>
          <DialogDescription>
            <Trans id='Share' />{' '}
            <span className='mono text-foreground'>{share?.metadata.name}</span>{' '}
            <Trans id='will be unexported. Data is untouched.' />
          </DialogDescription>
        </DialogHeader>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button
            variant='danger'
            disabled={del.isPending}
            onClick={async () => {
              if (!share) return;
              try {
                await del.mutateAsync(share.metadata.name);
                toast.success(i18n._('Share deleted'), share.metadata.name);
                onOpenChange(false);
              } catch (err) {
                toast.error(i18n._('Failed to delete share'), (err as Error)?.message);
              }
            }}
          >
            {del.isPending ? <Trans id='Deleting…' /> : <Trans id='Delete' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function ShareDetailDrawer({ share, onClose }: { share: Share | null; onClose: () => void }) {
  return (
    <Dialog open={!!share} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{share?.metadata.name}</DialogTitle>
          <DialogDescription>
            <span className='mono'>{share?.spec.dataset}</span> ·{' '}
            <span className='mono'>{share?.spec.path}</span>
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3 text-xs'>
          <div>
            <div className='text-foreground-subtle uppercase tracking-wider mb-1'>
              <Trans id='Bound servers' />
            </div>
            <ul className='flex flex-col gap-0.5 mono'>
              {share?.spec.protocols.smb?.server && (
                <li>SMB → {share.spec.protocols.smb.server}</li>
              )}
              {share?.spec.protocols.nfs?.server && (
                <li>NFS → {share.spec.protocols.nfs.server}</li>
              )}
            </ul>
          </div>
          <div>
            <div className='text-foreground-subtle uppercase tracking-wider mb-1'>
              <Trans id='Status' />
            </div>
            <div className='mono'>
              <Trans id='Phase' />: {share?.status?.phase ?? 'Pending'}
            </div>
            {share?.status?.exportedAt && (
              <div className='mono'>
                <Trans id='Exported' />: {share.status.exportedAt}
              </div>
            )}
          </div>
          <div>
            <div className='text-foreground-subtle uppercase tracking-wider mb-1'>
              <Trans id='Access' /> ({share?.spec.access?.length ?? 0})
            </div>
            <ul className='flex flex-col gap-0.5 mono'>
              {share?.spec.access?.map((a, i) => (
                <li key={i}>
                  {a.principal.user ? `user:${a.principal.user}` : `group:${a.principal.group}`} →{' '}
                  {a.mode}
                </li>
              ))}
            </ul>
          </div>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={onClose}>
            <Trans id='Close' />
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
