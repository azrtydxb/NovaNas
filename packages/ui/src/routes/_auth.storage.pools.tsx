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
import { createFileRoute } from '@tanstack/react-router';
import { Database, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
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
        subtitle={i18n._('Performance tiers (1 = fastest, 4 = slowest) backed by the chunk engine.')}
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
  const toast = useToast();
  const form = useForm<CreatePoolForm>({
    resolver: zodResolver(CreatePoolFormSchema),
    mode: 'onChange',
    defaultValues: { name: '', tier: '1' },
  });

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
                    {tierOptions.map((t) => (
                      <SelectItem key={t} value={t}>
                        Tier {t}
                      </SelectItem>
                    ))}
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
