// TODO(i18n-wave-12): strings on this page are still raw English. Migrate to <Trans>/i18n._() once wave 12 is green.
import { useDatasets } from '@/api/datasets';
import {
  type SnapshotCreateBody,
  useCreateSnapshot,
  useDeleteSnapshot,
  useSnapshots,
} from '@/api/snapshots';
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
import { formatBytes } from '@/lib/format';
import { maybeTrackJobFromResponse } from '@/stores/jobs';
import { zodResolver } from '@hookform/resolvers/zod';
import type { Snapshot, VolumeSourceRef } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { Camera, Plus, Trash2 } from 'lucide-react';
import { useMemo, useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { z } from 'zod';

export const Route = createFileRoute('/_auth/storage/snapshots')({
  component: SnapshotsPage,
});

const sourceKindOptions = ['Dataset', 'BlockVolume', 'Bucket', 'AppInstance', 'Vm'] as const;

const CreateSnapshotFormSchema = z.object({
  name: z
    .string()
    .min(1, 'Name required')
    .regex(/^[a-z0-9.-]+$/, 'lowercase letters, digits, dashes, dots only'),
  sourceKind: z.enum(sourceKindOptions),
  sourceName: z.string().min(1, 'Source required'),
  locked: z.boolean().optional(),
});
type CreateSnapshotForm = z.infer<typeof CreateSnapshotFormSchema>;

function SnapshotsPage() {
  const { canMutate } = useAuth();
  const mayMutate = canMutate();
  const [filterName, setFilterName] = useState<string>('');
  const source: VolumeSourceRef | undefined = useMemo(() => {
    if (!filterName) return undefined;
    return { kind: 'Dataset', name: filterName };
  }, [filterName]);

  const snaps = useSnapshots(source);
  const datasets = useDatasets();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Snapshot | null>(null);

  return (
    <>
      <PageHeader
        title='Snapshots'
        subtitle='Immutable, chunk-deduplicated. Retention via SnapshotSchedule.'
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> New snapshot
            </Button>
          ) : null
        }
      />

      <div className='flex items-center gap-2 mb-3'>
        <span className='text-xs text-foreground-subtle uppercase tracking-wider'>
          Filter by dataset
        </span>
        <Select
          value={filterName || '__all__'}
          onValueChange={(v) => setFilterName(v === '__all__' ? '' : v)}
        >
          <SelectTrigger className='w-[220px]'>
            <SelectValue placeholder='All' />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='__all__'>All sources</SelectItem>
            {datasets.data?.map((ds) => (
              <SelectItem key={ds.metadata.name} value={ds.metadata.name}>
                {ds.metadata.name}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>
      </div>

      {snaps.isLoading ? (
        <div className='flex flex-col gap-2'>
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className='h-9' />
          ))}
        </div>
      ) : snaps.isError ? (
        <EmptyState
          icon={<Camera size={28} />}
          title='Unable to load snapshots'
          description={(snaps.error as Error)?.message ?? 'Try again in a moment.'}
          action={<Button onClick={() => snaps.refetch()}>Retry</Button>}
        />
      ) : (snaps.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<Camera size={28} />}
          title='No snapshots'
          description='Snapshots are created manually or by a SnapshotSchedule.'
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> New snapshot
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
                <TableHeaderCell>Source</TableHeaderCell>
                <TableHeaderCell>Size</TableHeaderCell>
                <TableHeaderCell>Phase</TableHeaderCell>
                <TableHeaderCell>Created</TableHeaderCell>
                <TableHeaderCell>Locked</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {snaps.data!.map((s) => (
                <TableRow key={s.metadata.name}>
                  <TableCell className='mono'>{s.metadata.name}</TableCell>
                  <TableCell className='mono text-xs'>
                    {s.spec.source.kind}/{s.spec.source.name}
                  </TableCell>
                  <TableCell className='mono'>
                    {s.status?.sizeBytes != null ? formatBytes(s.status.sizeBytes) : '—'}
                  </TableCell>
                  <TableCell>
                    <Badge>{s.status?.phase ?? 'Pending'}</Badge>
                  </TableCell>
                  <TableCell className='mono text-xs text-foreground-muted'>
                    {s.status?.createdAt ?? s.metadata.creationTimestamp ?? '—'}
                  </TableCell>
                  <TableCell>{s.spec.locked ? <Badge>locked</Badge> : null}</TableCell>
                  <TableCell className='text-right'>
                    {mayMutate && !s.spec.locked && (
                      <Button size='sm' variant='danger' onClick={() => setDeleteTarget(s)}>
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

      <CreateSnapshotDialog open={createOpen} onOpenChange={setCreateOpen} />
      <DeleteSnapshotDialog
        snapshot={deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      />
    </>
  );
}

function CreateSnapshotDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateSnapshot();
  const toast = useToast();
  const datasets = useDatasets();
  const form = useForm<CreateSnapshotForm>({
    resolver: zodResolver(CreateSnapshotFormSchema),
    mode: 'onChange',
    defaultValues: { name: '', sourceKind: 'Dataset', sourceName: '', locked: false },
  });

  const kind = form.watch('sourceKind');

  const onSubmit = form.handleSubmit(async (values) => {
    const source: VolumeSourceRef =
      values.sourceKind === 'AppInstance' || values.sourceKind === 'Vm'
        ? { kind: values.sourceKind, name: values.sourceName, namespace: 'default' }
        : { kind: values.sourceKind, name: values.sourceName };
    const body: SnapshotCreateBody = {
      metadata: { name: values.name },
      spec: {
        source,
        ...(values.locked ? { locked: true } : {}),
      },
    };
    try {
      const resp = await create.mutateAsync(body);
      maybeTrackJobFromResponse(resp, `Snapshot ${values.name}`);
      toast.success('Snapshot created', values.name);
      form.reset();
      onOpenChange(false);
    } catch (err) {
      toast.error('Failed to create snapshot', (err as Error)?.message);
    }
  });

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New snapshot</DialogTitle>
          <DialogDescription>Capture a point-in-time copy of a source resource.</DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className='flex flex-col gap-3'>
          <FormField
            label='Name'
            htmlFor='snap-name'
            required
            error={form.formState.errors.name?.message}
          >
            <Input
              id='snap-name'
              placeholder='family-media.2026-04-22'
              {...form.register('name')}
            />
          </FormField>

          <FormField label='Source kind' required>
            <Controller
              control={form.control}
              name='sourceKind'
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {sourceKindOptions.map((k) => (
                      <SelectItem key={k} value={k}>
                        {k}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>

          <FormField label='Source name' required error={form.formState.errors.sourceName?.message}>
            {kind === 'Dataset' ? (
              <Controller
                control={form.control}
                name='sourceName'
                render={({ field }) => (
                  <Select value={field.value} onValueChange={field.onChange}>
                    <SelectTrigger>
                      <SelectValue placeholder='Select a dataset' />
                    </SelectTrigger>
                    <SelectContent>
                      {datasets.data?.map((ds) => (
                        <SelectItem key={ds.metadata.name} value={ds.metadata.name}>
                          {ds.metadata.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            ) : (
              <Input placeholder='resource name' {...form.register('sourceName')} />
            )}
          </FormField>

          <label className='flex items-center gap-2 text-sm text-foreground-muted'>
            <input type='checkbox' {...form.register('locked')} className='rounded' />
            Lock (prevents deletion until retention expires)
          </label>

          <DialogFooter>
            <Button type='button' variant='ghost' onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type='submit'
              variant='primary'
              disabled={!form.formState.isDirty || !form.formState.isValid || create.isPending}
            >
              {create.isPending ? 'Creating…' : 'Create snapshot'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function DeleteSnapshotDialog({
  snapshot,
  onOpenChange,
}: {
  snapshot: Snapshot | null;
  onOpenChange: (v: boolean) => void;
}) {
  const del = useDeleteSnapshot();
  const toast = useToast();
  return (
    <Dialog open={!!snapshot} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete snapshot?</DialogTitle>
          <DialogDescription>
            <span className='mono text-foreground'>{snapshot?.metadata.name}</span> will be removed.
            Dependent replications may be affected.
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
              if (!snapshot) return;
              try {
                await del.mutateAsync(snapshot.metadata.name);
                toast.success('Snapshot deleted', snapshot.metadata.name);
                onOpenChange(false);
              } catch (err) {
                toast.error('Failed to delete snapshot', (err as Error)?.message);
              }
            }}
          >
            {del.isPending ? 'Deleting…' : 'Delete snapshot'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
