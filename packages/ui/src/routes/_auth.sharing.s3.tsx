// TODO(i18n-wave-12): strings on this page are still raw English. Migrate to <Trans>/i18n._() once wave 12 is green.
import { useBucketUsers } from '@/api/bucket-users';
import { type BucketCreateBody, useBuckets, useCreateBucket, useDeleteBucket } from '@/api/buckets';
import { useObjectStores } from '@/api/object-stores';
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
import { zodResolver } from '@hookform/resolvers/zod';
import type { Bucket, BucketSpec } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { Cloud, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';
import { Controller, useForm } from 'react-hook-form';
import { z } from 'zod';

export const Route = createFileRoute('/_auth/sharing/s3')({
  component: S3Page,
});

const versioningOptions = ['disabled', 'enabled', 'suspended'] as const;
const objectLockModeOptions = ['governance', 'compliance'] as const;
const protectionOptions = ['none', 'replication', 'erasureCoding'] as const;

const CreateBucketFormSchema = z.object({
  name: z
    .string()
    .min(1, 'Name required')
    .regex(/^[a-z0-9-]+$/, 'lowercase letters, digits and dashes only'),
  store: z.string().min(1, 'ObjectStore required'),
  protection: z.enum(protectionOptions),
  replicationCopies: z.coerce.number().int().min(1).max(8).optional(),
  ecData: z.coerce.number().int().min(2).optional(),
  ecParity: z.coerce.number().int().min(1).optional(),
  versioning: z.enum(versioningOptions),
  encryptionEnabled: z.boolean(),
  objectLockEnabled: z.boolean(),
  objectLockMode: z.enum(objectLockModeOptions).optional(),
  quotaBytes: z.string().optional(),
});
type CreateBucketForm = z.infer<typeof CreateBucketFormSchema>;

function S3Page() {
  const { canMutate } = useAuth();
  const buckets = useBuckets();
  const [createOpen, setCreateOpen] = useState(false);
  const [deleteTarget, setDeleteTarget] = useState<Bucket | null>(null);
  const [selected, setSelected] = useState<Bucket | null>(null);
  const [showUsers, setShowUsers] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title='S3 Buckets'
        subtitle='Native chunk-engine object storage with object lock, versioning, and lifecycle.'
        actions={
          <div className='flex gap-2'>
            <Button variant='ghost' onClick={() => setShowUsers(true)}>
              Bucket users
            </Button>
            {mayMutate && (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> Create bucket
              </Button>
            )}
          </div>
        }
      />

      {buckets.isLoading ? (
        <div className='flex flex-col gap-2'>
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className='h-9' />
          ))}
        </div>
      ) : buckets.isError ? (
        <EmptyState
          icon={<Cloud size={28} />}
          title='Unable to load buckets'
          description={(buckets.error as Error)?.message ?? 'Try again in a moment.'}
          action={<Button onClick={() => buckets.refetch()}>Retry</Button>}
        />
      ) : (buckets.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<Cloud size={28} />}
          title='No buckets yet'
          description='Create a bucket on an ObjectStore to expose S3-compatible storage.'
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> Create bucket
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
                <TableHeaderCell>Store</TableHeaderCell>
                <TableHeaderCell>Versioning</TableHeaderCell>
                <TableHeaderCell>Object lock</TableHeaderCell>
                <TableHeaderCell>Encryption</TableHeaderCell>
                <TableHeaderCell>Quota</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {buckets.data!.map((b) => {
                const phase = b.status?.phase ?? 'Pending';
                const tone = phase === 'Active' ? 'ok' : phase === 'Failed' ? 'err' : 'idle';
                const lock = b.spec.objectLock;
                return (
                  <TableRow
                    key={b.metadata.name}
                    className='cursor-pointer'
                    onClick={() => setSelected(b)}
                  >
                    <TableCell>
                      <StatusDot tone={tone} className='mr-2' />
                      <span className='text-foreground font-medium'>{b.metadata.name}</span>
                    </TableCell>
                    <TableCell className='mono text-xs'>{b.spec.store}</TableCell>
                    <TableCell>
                      <Badge>{b.spec.versioning ?? 'disabled'}</Badge>
                    </TableCell>
                    <TableCell>
                      {lock?.enabled ? (
                        <Badge>{lock.mode ?? 'on'}</Badge>
                      ) : (
                        <span className='text-foreground-subtle text-xs'>—</span>
                      )}
                    </TableCell>
                    <TableCell>
                      {b.spec.encryption?.enabled ? (
                        <Badge>on</Badge>
                      ) : (
                        <span className='text-foreground-subtle text-xs'>—</span>
                      )}
                    </TableCell>
                    <TableCell className='mono text-xs'>{b.spec.quota?.hardBytes ?? '—'}</TableCell>
                    <TableCell className='text-right'>
                      {mayMutate && (
                        <Button
                          size='sm'
                          variant='danger'
                          onClick={(e) => {
                            e.stopPropagation();
                            setDeleteTarget(b);
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

      <CreateBucketDialog open={createOpen} onOpenChange={setCreateOpen} />
      <DeleteBucketDialog
        bucket={deleteTarget}
        onOpenChange={(open) => !open && setDeleteTarget(null)}
      />
      <BucketDetailDrawer bucket={selected} onClose={() => setSelected(null)} />
      <BucketUsersDialog open={showUsers} onOpenChange={setShowUsers} />
    </>
  );
}

// -----------------------------------------------------------------------------
function CreateBucketDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateBucket();
  const stores = useObjectStores();
  const toast = useToast();
  const form = useForm<CreateBucketForm>({
    resolver: zodResolver(CreateBucketFormSchema),
    mode: 'onChange',
    defaultValues: {
      name: '',
      store: '',
      protection: 'none',
      versioning: 'disabled',
      encryptionEnabled: false,
      objectLockEnabled: false,
    },
  });

  const onSubmit = form.handleSubmit(async (values) => {
    const spec: BucketSpec = {
      store: values.store,
      versioning: values.versioning,
      encryption: { enabled: values.encryptionEnabled },
      objectLock: values.objectLockEnabled
        ? { enabled: true, mode: values.objectLockMode }
        : { enabled: false },
    };
    if (values.protection === 'replication' && values.replicationCopies) {
      spec.protection = {
        mode: 'replication',
        replication: { copies: values.replicationCopies },
      };
    } else if (values.protection === 'erasureCoding' && values.ecData && values.ecParity) {
      spec.protection = {
        mode: 'erasureCoding',
        erasureCoding: {
          dataShards: values.ecData,
          parityShards: values.ecParity,
        },
      };
    }
    if (values.quotaBytes) {
      spec.quota = { hardBytes: values.quotaBytes };
    }
    const body: BucketCreateBody = { metadata: { name: values.name }, spec };
    try {
      await create.mutateAsync(body);
      toast.success('Bucket created', values.name);
      form.reset();
      onOpenChange(false);
    } catch (err) {
      toast.error('Failed to create bucket', (err as Error)?.message);
    }
  });

  const protection = form.watch('protection');
  const objectLockEnabled = form.watch('objectLockEnabled');

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Create bucket</DialogTitle>
          <DialogDescription>Expose an S3 bucket on an existing ObjectStore.</DialogDescription>
        </DialogHeader>
        <form onSubmit={onSubmit} className='flex flex-col gap-3'>
          <FormField
            label='Name'
            htmlFor='bucket-name'
            required
            error={form.formState.errors.name?.message}
          >
            <Input id='bucket-name' placeholder='media' {...form.register('name')} />
          </FormField>

          <FormField label='ObjectStore' required error={form.formState.errors.store?.message}>
            <Controller
              control={form.control}
              name='store'
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger>
                    <SelectValue placeholder={stores.isLoading ? 'Loading…' : 'Select a store'} />
                  </SelectTrigger>
                  <SelectContent>
                    {stores.data?.map((s) => (
                      <SelectItem key={s.metadata.name} value={s.metadata.name}>
                        {s.metadata.name}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>

          <FormField label='Protection'>
            <Controller
              control={form.control}
              name='protection'
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='none'>Default</SelectItem>
                    <SelectItem value='replication'>Replication</SelectItem>
                    <SelectItem value='erasureCoding'>Erasure coding</SelectItem>
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>

          {protection === 'replication' && (
            <FormField label='Copies'>
              <Input type='number' min={1} max={8} {...form.register('replicationCopies')} />
            </FormField>
          )}
          {protection === 'erasureCoding' && (
            <div className='grid grid-cols-2 gap-3'>
              <FormField label='Data shards'>
                <Input type='number' min={2} {...form.register('ecData')} />
              </FormField>
              <FormField label='Parity shards'>
                <Input type='number' min={1} {...form.register('ecParity')} />
              </FormField>
            </div>
          )}

          <FormField label='Versioning'>
            <Controller
              control={form.control}
              name='versioning'
              render={({ field }) => (
                <Select value={field.value} onValueChange={field.onChange}>
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    {versioningOptions.map((v) => (
                      <SelectItem key={v} value={v}>
                        {v}
                      </SelectItem>
                    ))}
                  </SelectContent>
                </Select>
              )}
            />
          </FormField>

          <Controller
            control={form.control}
            name='encryptionEnabled'
            render={({ field }) => (
              <div className='flex items-center gap-2 text-sm'>
                <Checkbox checked={field.value} onCheckedChange={(v) => field.onChange(!!v)} />
                Encryption at rest
              </div>
            )}
          />
          <Controller
            control={form.control}
            name='objectLockEnabled'
            render={({ field }) => (
              <div className='flex items-center gap-2 text-sm'>
                <Checkbox checked={field.value} onCheckedChange={(v) => field.onChange(!!v)} />
                Enable object lock
              </div>
            )}
          />
          {objectLockEnabled && (
            <FormField label='Object lock mode' hint='Compliance is permanent — cannot be reduced.'>
              <Controller
                control={form.control}
                name='objectLockMode'
                render={({ field }) => (
                  <Select value={field.value ?? ''} onValueChange={field.onChange}>
                    <SelectTrigger>
                      <SelectValue placeholder='Select mode' />
                    </SelectTrigger>
                    <SelectContent>
                      {objectLockModeOptions.map((m) => (
                        <SelectItem key={m} value={m}>
                          {m}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                )}
              />
            </FormField>
          )}

          <FormField label='Quota (bytes)' hint='Optional — e.g. 1Ti'>
            <Input placeholder='1Ti' {...form.register('quotaBytes')} />
          </FormField>

          <DialogFooter>
            <Button type='button' variant='ghost' onClick={() => onOpenChange(false)}>
              Cancel
            </Button>
            <Button
              type='submit'
              variant='primary'
              disabled={!form.formState.isValid || create.isPending}
            >
              {create.isPending ? 'Creating…' : 'Create bucket'}
            </Button>
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}

function DeleteBucketDialog({
  bucket,
  onOpenChange,
}: {
  bucket: Bucket | null;
  onOpenChange: (v: boolean) => void;
}) {
  const del = useDeleteBucket();
  const toast = useToast();
  const compliance =
    bucket?.spec.objectLock?.enabled && bucket.spec.objectLock.mode === 'compliance';
  const governance =
    bucket?.spec.objectLock?.enabled && bucket.spec.objectLock.mode === 'governance';

  return (
    <Dialog open={!!bucket} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete bucket?</DialogTitle>
          <DialogDescription>
            Bucket <span className='mono text-foreground'>{bucket?.metadata.name}</span> and all
            objects in it will be removed.
          </DialogDescription>
        </DialogHeader>
        {compliance && (
          <div className='text-xs text-danger border border-danger/40 rounded-md p-2'>
            This bucket is under <strong>compliance</strong> object lock — deletion is refused.
          </div>
        )}
        {governance && !compliance && (
          <div className='text-xs text-warning border border-warning/40 rounded-md p-2'>
            This bucket is under <strong>governance</strong> object lock. Retained objects may block
            deletion until their retention period expires.
          </div>
        )}
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant='danger'
            disabled={del.isPending || compliance}
            onClick={async () => {
              if (!bucket) return;
              try {
                await del.mutateAsync(bucket.metadata.name);
                toast.success('Bucket deleted', bucket.metadata.name);
                onOpenChange(false);
              } catch (err) {
                toast.error('Failed to delete bucket', (err as Error)?.message);
              }
            }}
          >
            {del.isPending ? 'Deleting…' : 'Delete bucket'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function BucketDetailDrawer({ bucket, onClose }: { bucket: Bucket | null; onClose: () => void }) {
  const users = useBucketUsers();
  const open = !!bucket;
  const linkedUsers =
    users.data?.filter((u) => u.spec.policies?.some((p) => p.bucket === bucket?.metadata.name)) ??
    [];
  return (
    <Dialog open={open} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{bucket?.metadata.name}</DialogTitle>
          <DialogDescription>
            Store: <span className='mono'>{bucket?.spec.store}</span> · Phase:{' '}
            <span className='mono'>{bucket?.status?.phase ?? 'Pending'}</span>
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3 text-xs'>
          <div className='grid grid-cols-2 gap-2'>
            <Stat label='Objects' value={bucket?.status?.objectCount ?? 0} />
            <Stat label='Used bytes' value={bucket?.status?.usedBytes ?? 0} />
          </div>
          <div>
            <div className='text-foreground-subtle uppercase tracking-wider mb-1'>
              Lifecycle rules
            </div>
            {(bucket?.spec.lifecycle?.length ?? 0) === 0 ? (
              <div className='text-foreground-subtle'>No lifecycle rules.</div>
            ) : (
              <ul className='flex flex-col gap-0.5 mono'>
                {bucket!.spec.lifecycle!.map((r, i) => (
                  <li key={i}>
                    {r.prefix ?? '*'} {r.expireAfter ? `→ expire ${r.expireAfter}` : ''}{' '}
                    {r.transitionAfter ? `→ ${r.transitionTo} after ${r.transitionAfter}` : ''}
                  </li>
                ))}
              </ul>
            )}
          </div>
          <div>
            <div className='text-foreground-subtle uppercase tracking-wider mb-1'>Bucket users</div>
            {linkedUsers.length === 0 ? (
              <div className='text-foreground-subtle'>No users scoped to this bucket.</div>
            ) : (
              <ul className='flex flex-col gap-0.5 mono'>
                {linkedUsers.map((u) => (
                  <li key={u.metadata.name}>
                    {u.metadata.name} — {u.status?.accessKeyId ?? '—'}
                  </li>
                ))}
              </ul>
            )}
          </div>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={onClose}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function Stat({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div className='border border-border rounded-md p-2'>
      <div className='text-foreground-subtle uppercase tracking-wider text-[10px]'>{label}</div>
      <div className='mono text-foreground'>{value}</div>
    </div>
  );
}

function BucketUsersDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const users = useBucketUsers();
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Bucket users</DialogTitle>
          <DialogDescription>S3 credentials scoped to buckets and prefixes.</DialogDescription>
        </DialogHeader>
        {users.isLoading ? (
          <Skeleton className='h-9' />
        ) : users.isError ? (
          <div className='text-xs text-danger'>Failed to load bucket users.</div>
        ) : (users.data?.length ?? 0) === 0 ? (
          <div className='text-sm text-foreground-subtle'>No bucket users yet.</div>
        ) : (
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>Name</TableHeaderCell>
                <TableHeaderCell>Access key</TableHeaderCell>
                <TableHeaderCell>Policies</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {users.data!.map((u) => (
                <TableRow key={u.metadata.name}>
                  <TableCell>{u.metadata.name}</TableCell>
                  <TableCell className='mono text-xs'>{u.status?.accessKeyId ?? '—'}</TableCell>
                  <TableCell className='mono text-xs'>{u.spec.policies?.length ?? 0}</TableCell>
                </TableRow>
              ))}
            </TableBody>
          </Table>
        )}
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Close
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
