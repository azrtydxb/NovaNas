import { api } from '@/api/client';
import { useDisks } from '@/api/disks';
import {
  type PoolCreateBody,
  type PoolUpdateBody,
  useCreatePool,
  useDeletePool,
  usePools,
  useUpdatePool,
} from '@/api/pools';
import { CapacityBar } from '@/components/common/capacity-bar';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { StatusDot } from '@/components/common/status-dot';
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
import { zodResolver } from '@hookform/resolvers/zod';
import { Trans } from '@lingui/react';
import type { StoragePool } from '@novanas/schemas';
import { useQueryClient } from '@tanstack/react-query';
import { createFileRoute } from '@tanstack/react-router';
import { Database, Plus, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { z } from 'zod';

export const Route = createFileRoute('/_auth/storage/pools')({
  component: PoolsPage,
});

// -----------------------------------------------------------------------------
// Schema for the create form. We use a UI-local shape that maps to PoolCreateBody.
// -----------------------------------------------------------------------------
// Numeric performance tiers — must match the StoragePool CRD enum
// (packages/operators/config/crd/bases/novanas.io_storagepools.yaml).
// 1 is the fastest (NVMe / hot), 4 is the slowest (cold archive).
const tierOptions = ['1', '2', '3', '4'] as const;
const deviceClassOptions = ['nvme', 'ssd', 'hdd'] as const;

const CreatePoolFormSchema = z.object({
  name: z
    .string()
    .min(1, 'Name required')
    .regex(/^[a-z0-9-]+$/, 'lowercase letters, digits and dashes only'),
  tier: z.enum(tierOptions),
  deviceClass: z.enum(deviceClassOptions).optional(),
});
type CreatePoolForm = z.infer<typeof CreatePoolFormSchema>;

// -----------------------------------------------------------------------------

function PoolsPage() {
  const { canMutate } = useAuth();
  const pools = usePools();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<StoragePool | null>(null);
  const [selected, setSelected] = useState<StoragePool | null>(null);
  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title={i18n._('Pools')}
        subtitle={i18n._(
          'Performance tiers (1 = fastest, 4 = slowest) backed by the chunk engine.'
        )}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='Create pool' />
            </Button>
          ) : null
        }
      />

      {pools.isLoading ? (
        <div className='flex flex-col gap-2'>
          <Skeleton className='h-9' />
          <Skeleton className='h-9' />
          <Skeleton className='h-9' />
        </div>
      ) : pools.isError ? (
        <EmptyState
          icon={<Database size={28} />}
          title={i18n._('Unable to load pools')}
          description={(pools.error as Error)?.message ?? i18n._('Try again in a moment.')}
          action={<Button onClick={() => pools.refetch()}>{i18n._('Retry')}</Button>}
        />
      ) : (pools.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<Database size={28} />}
          title={i18n._('No pools yet')}
          description={i18n._('Create your first pool to start allocating storage.')}
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> <Trans id='Create pool' />
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
                  <Trans id='Tier' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Phase' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Disks' />
                </TableHeaderCell>
                <TableHeaderCell>
                  <Trans id='Usage' />
                </TableHeaderCell>
                <TableHeaderCell className='text-right'>
                  <Trans id='Actions' />
                </TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {pools.data!.map((p) => {
                const phase = p.status?.phase ?? 'Pending';
                const tone =
                  phase === 'Active'
                    ? 'ok'
                    : phase === 'Degraded'
                      ? 'warn'
                      : phase === 'Failed'
                        ? 'err'
                        : 'idle';
                const cap = p.status?.capacity;
                return (
                  <TableRow
                    key={p.metadata.name}
                    className='cursor-pointer'
                    onClick={() => setSelected(p)}
                  >
                    <TableCell>
                      <StatusDot tone={tone} className='mr-2' />
                      <span className='text-foreground font-medium'>{p.metadata.name}</span>
                    </TableCell>
                    <TableCell>
                      <Badge>Tier {p.spec.tier}</Badge>
                    </TableCell>
                    <TableCell>
                      <span className='mono text-xs text-foreground-muted'>{phase}</span>
                    </TableCell>
                    <TableCell className='mono'>{p.status?.diskCount ?? 0}</TableCell>
                    <TableCell>
                      {cap ? (
                        <CapacityBar used={cap.usedBytes} total={cap.totalBytes} />
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
                            setDeleteTarget(p);
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

      <CreatePoolDialog open={createOpen} onOpenChange={setCreateOpen} />
      <DeletePoolDialog
        pool={deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      />
      <PoolDetailDrawer pool={selected} onClose={() => setSelected(null)} />
    </>
  );
}

// -----------------------------------------------------------------------------
// Create dialog
// -----------------------------------------------------------------------------
function CreatePoolDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreatePool();
  const existing = usePools();
  const toast = useToast();
  // Tiers are exclusive — each pool owns a distinct performance level.
  // Filter the dropdown to only the tiers that aren't already taken,
  // and pick the lowest-numbered free tier as the default.
  const usedTiers = new Set((existing.data ?? []).map((p) => p.spec?.tier));
  const freeTiers = tierOptions.filter((t) => !usedTiers.has(t));
  const defaultTier = freeTiers[0] ?? tierOptions[0];
  const form = useForm<CreatePoolForm>({
    resolver: zodResolver(CreatePoolFormSchema),
    mode: 'onChange',
    defaultValues: { name: '', tier: defaultTier },
  });
  // Re-seat the default whenever the set of used tiers changes (e.g. a
  // pool got created/deleted while the dialog was closed). useForm's
  // defaultValues only run on mount, so we patch the field manually.
  useEffect(() => {
    if (open && form.getValues('tier') && usedTiers.has(form.getValues('tier'))) {
      form.setValue('tier', defaultTier, { shouldValidate: true });
    }
  }, [open, defaultTier, form, usedTiers]);

  const onSubmit = form.handleSubmit(async (values) => {
    const body: PoolCreateBody = {
      metadata: { name: values.name },
      spec: {
        tier: values.tier,
        ...(values.deviceClass ? { deviceFilter: { preferredClass: values.deviceClass } } : {}),
      },
    };
    try {
      await create.mutateAsync(body);
      toast.success('Pool created', values.name);
      form.reset();
      onOpenChange(false);
    } catch (err) {
      toast.error('Failed to create pool', (err as Error)?.message);
    }
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create pool</DialogTitle>
          <DialogDescription>Allocate storage capacity into a named tier.</DialogDescription>
        </DialogHeader>

        <form onSubmit={onSubmit} className='flex flex-col gap-3'>
          <FormField
            label='Name'
            htmlFor='pool-name'
            required
            error={form.formState.errors.name?.message}
          >
            <Input id='pool-name' placeholder='nvme-hot' {...form.register('name')} />
          </FormField>

          <FormField
            label='Tier'
            required
            hint='1 is the fastest (NVMe / hot data); 4 is the slowest (cold archive).'
            error={form.formState.errors.tier?.message}
          >
            <Controller
              control={form.control}
              name='tier'
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {tierOptions.map((t) => {
                      const taken = usedTiers.has(t);
                      return (
                        <SelectItem key={t} value={t} disabled={taken}>
                          Tier {t}
                          {taken ? ' · in use' : ''}
                        </SelectItem>
                      );
                    })}
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>

          <FormField label='Device class' hint='Optional — restricts disks placed into the pool.'>
            <Controller
              control={form.control}
              name='deviceClass'
              render={({ field }) => (
                <Select
                  value={field.value ?? ''}
                  onValueChange={(v) => field.onChange(v || undefined)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder='Any' />
                  </SelectTrigger>
                  <SelectContent>
                    {deviceClassOptions.map((d) => (
                      <SelectItem key={d} value={d}>
                        {d}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>

          <DialogFooter>
            <Button type='button' variant='ghost' onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type='submit'
              variant='primary'
              disabled={!form.formState.isDirty || !form.formState.isValid || create.isPending}
            >
              {create.isPending ? 'Creating…' : 'Create pool'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// -----------------------------------------------------------------------------
// Delete confirm
// -----------------------------------------------------------------------------
function DeletePoolDialog({
  pool,
  onOpenChange,
}: {
  pool: StoragePool | null;
  onOpenChange: (v: boolean) => void;
}) {
  const del = useDeletePool();
  const toast = useToast();
  const open = !!pool;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete pool?</DialogTitle>
          <DialogDescription>
            Pool <span className='mono text-foreground'>{pool?.metadata.name}</span> and all
            datasets on it will be removed. This cannot be undone.
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
              if (!pool) return;
              try {
                await del.mutateAsync(pool.metadata.name);
                toast.success('Pool deleted', pool.metadata.name);
                onOpenChange(false);
              } catch (err) {
                toast.error('Failed to delete pool', (err as Error)?.message);
              }
            }}
          >
            {del.isPending ? 'Deleting…' : 'Delete pool'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -----------------------------------------------------------------------------
// Detail drawer (as Dialog for now)
// -----------------------------------------------------------------------------
function PoolDetailDrawer({ pool, onClose }: { pool: StoragePool | null; onClose: () => void }) {
  const { canMutate } = useAuth();
  const mayMutate = canMutate();
  const update = useUpdatePool(pool?.metadata.name ?? '');
  const toast = useToast();
  const open = !!pool;
  const [tier, setTier] = useState<string>(pool?.spec.tier ?? 'hot');

  // Re-sync when a new pool is shown.
  if (pool && tier !== pool.spec.tier && update.isIdle) {
    setTier(pool.spec.tier);
  }

  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{pool?.metadata.name}</DialogTitle>
          <DialogDescription>
            Phase: <span className='mono'>{pool?.status?.phase ?? 'Pending'}</span> · Disks:{' '}
            <span className='mono'>{pool?.status?.diskCount ?? 0}</span>
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          {pool?.status?.capacity && (
            <CapacityBar
              used={pool.status.capacity.usedBytes}
              total={pool.status.capacity.totalBytes}
            />
          )}
          <FormField label='Tier'>
            <Select value={tier} onValueChange={setTier} disabled={!mayMutate}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                {tierOptions.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </FormField>
          {pool && <PoolDisksSection pool={pool} mayMutate={mayMutate} />}
          {pool?.status?.conditions && pool.status.conditions.length > 0 && (
            <div className='border border-border rounded-md p-2 text-xs'>
              <div className='text-foreground-subtle uppercase tracking-wider mb-1'>Conditions</div>
              <ul className='flex flex-col gap-0.5'>
                {pool.status.conditions.map((c) => (
                  <li key={c.type} className='mono'>
                    <span className='text-foreground'>{c.type}</span>:{' '}
                    <span className='text-foreground-muted'>{c.status}</span>
                    {c.message ? ` — ${c.message}` : ''}
                  </li>
                ))}
              </ul>
            </div>
          )}
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={onClose}>
            Close
          </Button>
          {mayMutate && (
            <Button
              variant='primary'
              disabled={!pool || tier === pool.spec.tier || update.isPending}
              onClick={async () => {
                if (!pool) return;
                try {
                  const body: PoolUpdateBody = {
                    spec: { tier: tier as PoolUpdateBody['spec']['tier'] },
                  };
                  await update.mutateAsync(body);
                  toast.success('Pool updated', pool.metadata.name);
                  onClose();
                } catch (err) {
                  toast.error('Failed to update pool', (err as Error)?.message);
                }
              }}
            >
              {update.isPending ? 'Saving…' : 'Save changes'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -----------------------------------------------------------------------------
// PoolDisksSection — list attached disks + attach unassigned disks.
// Disk-spec is the source of truth: `disk.spec.pool === pool.metadata.name`
// indicates membership. The DiskReconciler then transitions IDENTIFIED →
// ASSIGNED → ACTIVE; the StoragePoolReconciler aggregates capacity from the
// member set. So the UI only needs to PATCH each disk's spec.
// -----------------------------------------------------------------------------
function PoolDisksSection({ pool, mayMutate }: { pool: StoragePool; mayMutate: boolean }) {
  const disks = useDisks();
  const toast = useToast();
  const [picking, setPicking] = useState(false);
  const all = disks.data ?? [];
  const attached = all.filter((d) => d.spec?.pool === pool.metadata.name);
  // Exclude OS disks (novanas.io/system=true label, set by the
  // disk-agent for any disk hosting a host-mounted partition / swap).
  // The disk is still visible on the Disks page; it just isn't a
  // legal target for pool assignment.
  const isSystem = (d: import('@novanas/schemas').Disk): boolean =>
    d.metadata?.labels?.['novanas.io/system'] === 'true';
  // Honour the pool's deviceFilter.preferredClass — if the pool was
  // created as an HDD pool, don't let the admin attach SSD/NVMe disks
  // (and vice versa). Pools with no preference accept any class.
  const requiredClass = pool.spec?.deviceFilter?.preferredClass;
  const unassigned = all.filter((d) => {
    if (d.spec?.pool) return false;
    if (isSystem(d)) return false;
    if (requiredClass && d.status?.deviceClass !== requiredClass) return false;
    return true;
  });
  const fmtSize = (b?: number): string => {
    if (!b) return '—';
    const tb = b / 1e12;
    if (tb >= 1) return `${tb.toFixed(1)} TB`;
    return `${(b / 1e9).toFixed(1)} GB`;
  };
  return (
    <div className='border border-border rounded-md p-2 text-xs'>
      <div className='flex items-center justify-between mb-1.5'>
        <span className='text-foreground-subtle uppercase tracking-wider'>
          Disks ({attached.length})
        </span>
        {mayMutate && (
          <Button size='sm' variant='ghost' onClick={() => setPicking(true)}>
            <Plus size={13} /> Attach disks
          </Button>
        )}
      </div>
      {attached.length === 0 ? (
        <div className='text-foreground-muted py-2 text-center'>
          No disks attached. {mayMutate ? 'Click Attach disks to add some.' : ''}
        </div>
      ) : (
        <ul className='flex flex-col gap-0.5'>
          {attached.map((d) => (
            <li key={d.metadata.name} className='flex items-center justify-between mono'>
              <span>
                <span className='text-foreground'>{d.status?.slot ?? d.metadata.name}</span>
                <span className='text-foreground-muted'>
                  {' '}
                  · {d.status?.deviceClass ?? '?'} · {fmtSize(d.status?.sizeBytes)}
                </span>
              </span>
              <span className='text-foreground-subtle'>{d.status?.state ?? '—'}</span>
            </li>
          ))}
        </ul>
      )}
      <AttachDisksDialog
        open={picking}
        onClose={() => setPicking(false)}
        pool={pool}
        candidates={unassigned}
        onAttached={(n) => {
          toast.success(`Attached ${n} disk${n === 1 ? '' : 's'}`, `to pool ${pool.metadata.name}`);
          setPicking(false);
        }}
      />
    </div>
  );
}

function AttachDisksDialog({
  open,
  onClose,
  pool,
  candidates,
  onAttached,
}: {
  open: boolean;
  onClose: () => void;
  pool: StoragePool;
  candidates: import('@novanas/schemas').Disk[];
  onAttached: (count: number) => void;
}) {
  const [selected, setSelected] = useState<Set<string>>(new Set());
  const [submitting, setSubmitting] = useState(false);
  const toast = useToast();
  const qc = useQueryClient();
  const fmtSize = (b?: number): string => {
    if (!b) return '—';
    const tb = b / 1e12;
    if (tb >= 1) return `${tb.toFixed(1)} TB`;
    return `${(b / 1e9).toFixed(1)} GB`;
  };
  const toggle = (name: string) => {
    const next = new Set(selected);
    if (next.has(name)) next.delete(name);
    else next.add(name);
    setSelected(next);
  };
  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Attach disks to {pool.metadata.name}</DialogTitle>
          <DialogDescription>
            Select unassigned disks to add to this pool. They transition through IDENTIFIED →
            ASSIGNED → ACTIVE as the operator picks them up.
          </DialogDescription>
        </DialogHeader>
        {candidates.length === 0 ? (
          <div className='text-sm text-foreground-muted py-3 text-center'>
            No unassigned disks available.
          </div>
        ) : (
          <ul className='flex flex-col gap-1 max-h-[400px] overflow-y-auto'>
            {candidates.map((d) => (
              <li
                key={d.metadata.name}
                className='flex items-center gap-2 p-1.5 rounded-md hover:bg-elevated cursor-pointer'
                onClick={() => toggle(d.metadata.name)}
                onKeyDown={(e) => {
                  if (e.key === 'Enter' || e.key === ' ') {
                    e.preventDefault();
                    toggle(d.metadata.name);
                  }
                }}
              >
                <input
                  type='checkbox'
                  checked={selected.has(d.metadata.name)}
                  onChange={() => toggle(d.metadata.name)}
                  onClick={(e) => e.stopPropagation()}
                />
                <span className='mono text-xs'>
                  <span className='text-foreground'>{d.status?.slot ?? d.metadata.name}</span>
                  <span className='text-foreground-muted'>
                    {' '}
                    · {d.status?.deviceClass ?? '?'} · {fmtSize(d.status?.sizeBytes)} ·{' '}
                    {d.status?.model ?? ''}
                  </span>
                </span>
              </li>
            ))}
          </ul>
        )}
        <DialogFooter>
          <Button variant='ghost' onClick={onClose}>
            Cancel
          </Button>
          <Button
            variant='primary'
            disabled={selected.size === 0 || submitting}
            onClick={async () => {
              const names = Array.from(selected);
              setSubmitting(true);
              try {
                // Bulk-patch each selected disk's spec. The api hook
                // `useUpdateDisk` is keyed by a single name; for a
                // multi-select we hit the api endpoint directly so
                // every patch is independent.
                await Promise.all(
                  names.map((n) =>
                    api
                      .patch(`/disks/${encodeURIComponent(n)}`, {
                        spec: { pool: pool.metadata.name, role: 'data' },
                      })
                      .catch((e: unknown) => {
                        throw new Error(`${n}: ${(e as Error).message}`);
                      })
                  )
                );
                qc.invalidateQueries({ queryKey: ['disks'] });
                qc.invalidateQueries({ queryKey: ['pools'] });
                qc.invalidateQueries({ queryKey: ['pool', pool.metadata.name] });
                onAttached(names.length);
              } catch (err) {
                toast.error('Failed to attach disks', (err as Error)?.message);
              } finally {
                setSubmitting(false);
              }
            }}
          >
            {submitting ? 'Attaching…' : `Attach ${selected.size || ''}`}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
