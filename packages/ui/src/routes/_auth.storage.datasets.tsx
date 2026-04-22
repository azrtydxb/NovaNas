import {
  type DatasetCreateBody,
  useCreateDataset,
  useDatasets,
  useDeleteDataset,
  useUpdateDataset,
} from '@/api/datasets';
import { usePools } from '@/api/pools';
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
import type { Dataset, ProtectionPolicy } from '@novanas/schemas';
import { BytesQuantitySchema, ProtectionPolicySchema } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { FolderTree, Plus, Trash2 } from 'lucide-react';
import { useEffect, useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { z } from 'zod';

export const Route = createFileRoute('/_auth/storage/datasets')({
  component: DatasetsPage,
});

const filesystemOptions = ['xfs', 'ext4'] as const;
const compressionOptions = ['none', 'zstd', 'lz4', 'gzip'] as const;

const CreateDatasetFormSchema = z.object({
  name: z
    .string()
    .min(1, 'Name required')
    .regex(/^[a-z0-9-]+$/, 'lowercase letters, digits and dashes only'),
  pool: z.string().min(1, 'Pool required'),
  size: BytesQuantitySchema,
  filesystem: z.enum(filesystemOptions),
  compression: z.enum(compressionOptions).optional(),
});
type CreateDatasetForm = z.infer<typeof CreateDatasetFormSchema>;

function DatasetsPage() {
  const { canMutate } = useAuth();
  const datasets = useDatasets();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Dataset | null>(null);
  const [selected, setSelected] = useState<Dataset | null>(null);
  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title={i18n._('Datasets')}
        subtitle={i18n._('Logical volumes backed by pools, with their own protection and quota.')}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New dataset' />
            </Button>
          ) : null
        }
      />

      {datasets.isLoading ? (
        <div className='flex flex-col gap-2'>
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className='h-9' />
          ))}
        </div>
      ) : datasets.isError ? (
        <EmptyState
          icon={<FolderTree size={28} />}
          title='Unable to load datasets'
          description={(datasets.error as Error)?.message ?? 'Try again in a moment.'}
          action={<Button onClick={() => datasets.refetch()}>Retry</Button>}
        />
      ) : (datasets.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<FolderTree size={28} />}
          title='No datasets yet'
          description='Datasets are where your files, VMs and apps actually live.'
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> New dataset
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
                <TableHeaderCell>Pool</TableHeaderCell>
                <TableHeaderCell>Size</TableHeaderCell>
                <TableHeaderCell>Filesystem</TableHeaderCell>
                <TableHeaderCell>Protection</TableHeaderCell>
                <TableHeaderCell>Phase</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {datasets.data!.map((ds) => {
                const phase = ds.status?.phase ?? 'Pending';
                const tone =
                  phase === 'Mounted'
                    ? 'ok'
                    : phase === 'Degraded'
                      ? 'warn'
                      : phase === 'Failed'
                        ? 'err'
                        : 'idle';
                return (
                  <TableRow
                    key={ds.metadata.name}
                    className='cursor-pointer'
                    onClick={() => setSelected(ds)}
                  >
                    <TableCell>
                      <StatusDot tone={tone} className='mr-2' />
                      <span className='text-foreground font-medium'>{ds.metadata.name}</span>
                    </TableCell>
                    <TableCell className='mono text-xs'>{ds.spec.pool}</TableCell>
                    <TableCell className='mono'>{ds.spec.size}</TableCell>
                    <TableCell>
                      <Badge>{ds.spec.filesystem}</Badge>
                    </TableCell>
                    <TableCell>
                      <ProtectionBadge p={ds.spec.protection} />
                    </TableCell>
                    <TableCell className='mono text-xs text-foreground-muted'>{phase}</TableCell>
                    <TableCell className='text-right'>
                      {mayMutate && (
                        <Button
                          size='sm'
                          variant='danger'
                          onClick={(e) => {
                            e.stopPropagation();
                            setDeleteTarget(ds);
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

      <CreateDatasetDialog open={createOpen} onOpenChange={setCreateOpen} />
      <DeleteDatasetDialog
        dataset={deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      />
      <DatasetDetailDrawer dataset={selected} onClose={() => setSelected(null)} />
    </>
  );
}

function ProtectionBadge({ p }: { p?: ProtectionPolicy }) {
  if (!p) return <span className='text-foreground-subtle text-xs'>—</span>;
  if (p.mode === 'replication') {
    return <Badge>rep×{p.replication.copies}</Badge>;
  }
  return (
    <Badge>
      EC {p.erasureCoding.dataShards}+{p.erasureCoding.parityShards}
    </Badge>
  );
}

// -----------------------------------------------------------------------------
// Create dialog
// -----------------------------------------------------------------------------
function CreateDatasetDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateDataset();
  const pools = usePools();
  const toast = useToast();
  const form = useForm<CreateDatasetForm>({
    resolver: zodResolver(CreateDatasetFormSchema),
    mode: 'onChange',
    defaultValues: { name: '', pool: '', size: '100Gi', filesystem: 'xfs' },
  });

  const onSubmit = form.handleSubmit(async (values) => {
    const body: DatasetCreateBody = {
      metadata: { name: values.name },
      spec: {
        pool: values.pool,
        size: values.size,
        filesystem: values.filesystem,
        ...(values.compression ? { compression: values.compression } : {}),
      },
    };
    try {
      await create.mutateAsync(body);
      toast.success('Dataset created', values.name);
      form.reset();
      onOpenChange(false);
    } catch (err) {
      toast.error('Failed to create dataset', (err as Error)?.message);
    }
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New dataset</DialogTitle>
          <DialogDescription>Carve a filesystem out of an existing pool.</DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className='flex flex-col gap-3'>
          <FormField
            label='Name'
            htmlFor='ds-name'
            required
            error={form.formState.errors.name?.message}
          >
            <Input id='ds-name' placeholder='family-media' {...form.register('name')} />
          </FormField>

          <FormField label='Pool' required error={form.formState.errors.pool?.message}>
            <Controller
              control={form.control}
              name='pool'
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger>
                    <SelectValue placeholder={pools.isLoading ? 'Loading…' : 'Select a pool'} />
                  </SelectTrigger>
                  <SelectContent>
                    {pools.data?.map((p) => (
                      <SelectItem key={p.metadata.name} value={p.metadata.name}>
                        {p.metadata.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>

          <FormField
            label='Size'
            htmlFor='ds-size'
            required
            hint='Binary suffixes: 100Gi, 4Ti, …'
            error={form.formState.errors.size?.message}
          >
            <Input id='ds-size' placeholder='100Gi' {...form.register('size')} />
          </FormField>

          <FormField label='Filesystem' required error={form.formState.errors.filesystem?.message}>
            <Controller
              control={form.control}
              name='filesystem'
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {filesystemOptions.map((fs) => (
                      <SelectItem key={fs} value={fs}>
                        {fs}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>

          <FormField label='Compression' hint='Optional.'>
            <Controller
              control={form.control}
              name='compression'
              render={({ field }) => (
                <Select
                  value={field.value ?? ''}
                  onValueChange={(v) => field.onChange(v || undefined)}
                >
                  <SelectTrigger>
                    <SelectValue placeholder='Default' />
                  </SelectTrigger>
                  <SelectContent>
                    {compressionOptions.map((c) => (
                      <SelectItem key={c} value={c}>
                        {c}
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
              {create.isPending ? 'Creating…' : 'Create dataset'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

// -----------------------------------------------------------------------------
// Delete
// -----------------------------------------------------------------------------
function DeleteDatasetDialog({
  dataset,
  onOpenChange,
}: {
  dataset: Dataset | null;
  onOpenChange: (v: boolean) => void;
}) {
  const del = useDeleteDataset();
  const toast = useToast();
  return (
    <Dialog open={!!dataset} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete dataset?</DialogTitle>
          <DialogDescription>
            Dataset <span className='mono text-foreground'>{dataset?.metadata.name}</span> and its
            data will be removed. This cannot be undone.
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
              if (!dataset) return;
              try {
                await del.mutateAsync(dataset.metadata.name);
                toast.success('Dataset deleted', dataset.metadata.name);
                onOpenChange(false);
              } catch (err) {
                toast.error('Failed to delete dataset', (err as Error)?.message);
              }
            }}
          >
            {del.isPending ? 'Deleting…' : 'Delete dataset'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -----------------------------------------------------------------------------
// Detail drawer with protection policy editor
// -----------------------------------------------------------------------------
type ProtectionForm =
  | { mode: 'none' }
  | { mode: 'replication'; copies: number }
  | { mode: 'erasureCoding'; dataShards: number; parityShards: number };

function DatasetDetailDrawer({
  dataset,
  onClose,
}: { dataset: Dataset | null; onClose: () => void }) {
  const { canMutate } = useAuth();
  const mayMutate = canMutate();
  const toast = useToast();
  const update = useUpdateDataset(dataset?.metadata.name ?? '');

  const [form, setForm] = useState<ProtectionForm>({ mode: 'none' });

  useEffect(() => {
    if (!dataset) return;
    const p = dataset.spec.protection;
    if (!p) setForm({ mode: 'none' });
    else if (p.mode === 'replication')
      setForm({ mode: 'replication', copies: p.replication.copies });
    else
      setForm({
        mode: 'erasureCoding',
        dataShards: p.erasureCoding.dataShards,
        parityShards: p.erasureCoding.parityShards,
      });
  }, [dataset]);

  const save = async () => {
    if (!dataset) return;
    let protection: ProtectionPolicy | undefined;
    if (form.mode === 'replication') {
      protection = { mode: 'replication', replication: { copies: form.copies } };
    } else if (form.mode === 'erasureCoding') {
      protection = {
        mode: 'erasureCoding',
        erasureCoding: { dataShards: form.dataShards, parityShards: form.parityShards },
      };
    }
    if (protection) {
      const parsed = ProtectionPolicySchema.safeParse(protection);
      if (!parsed.success) {
        toast.error('Invalid protection policy', parsed.error.errors[0]?.message ?? '');
        return;
      }
    }
    try {
      await update.mutateAsync({ spec: { protection } as Partial<Dataset['spec']> });
      toast.success('Dataset updated', dataset.metadata.name);
      onClose();
    } catch (err) {
      toast.error('Failed to update dataset', (err as Error)?.message);
    }
  };

  return (
    <Dialog open={!!dataset} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{dataset?.metadata.name}</DialogTitle>
          <DialogDescription>
            {dataset?.spec.pool} · <span className='mono'>{dataset?.spec.size}</span> ·{' '}
            {dataset?.spec.filesystem}
          </DialogDescription>
        </DialogHeader>

        <div className='flex flex-col gap-3'>
          <div className='text-xs uppercase tracking-wider text-foreground-subtle'>
            Protection policy
          </div>

          <FormField label='Mode'>
            <Select
              value={form.mode}
              onValueChange={(v) => {
                if (v === 'none') setForm({ mode: 'none' });
                else if (v === 'replication') setForm({ mode: 'replication', copies: 2 });
                else setForm({ mode: 'erasureCoding', dataShards: 6, parityShards: 2 });
              }}
              disabled={!mayMutate}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='none'>None</SelectItem>
                <SelectItem value='replication'>Replication</SelectItem>
                <SelectItem value='erasureCoding'>Erasure coding</SelectItem>
              </SelectContent>
            </Select>
          </FormField>

          {form.mode === 'replication' && (
            <FormField label='Copies'>
              <Input
                type='number'
                min={1}
                max={8}
                value={form.copies}
                disabled={!mayMutate}
                onChange={(e) => setForm({ mode: 'replication', copies: Number(e.target.value) })}
              />
            </FormField>
          )}

          {form.mode === 'erasureCoding' && (
            <div className='grid grid-cols-2 gap-3'>
              <FormField label='Data shards'>
                <Input
                  type='number'
                  min={2}
                  value={form.dataShards}
                  disabled={!mayMutate}
                  onChange={(e) =>
                    setForm({
                      mode: 'erasureCoding',
                      dataShards: Number(e.target.value),
                      parityShards: form.parityShards,
                    })
                  }
                />
              </FormField>
              <FormField label='Parity shards'>
                <Input
                  type='number'
                  min={1}
                  value={form.parityShards}
                  disabled={!mayMutate}
                  onChange={(e) =>
                    setForm({
                      mode: 'erasureCoding',
                      dataShards: form.dataShards,
                      parityShards: Number(e.target.value),
                    })
                  }
                />
              </FormField>
            </div>
          )}
        </div>

        <DialogFooter>
          <Button variant='ghost' onClick={onClose}>
            Close
          </Button>
          {mayMutate && (
            <Button variant='primary' disabled={update.isPending} onClick={save}>
              {update.isPending ? 'Saving…' : 'Save policy'}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
