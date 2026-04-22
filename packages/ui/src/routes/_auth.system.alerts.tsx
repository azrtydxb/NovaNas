// TODO(i18n-wave-12): strings on this page are still raw English. Migrate to <Trans>/i18n._() once wave 12 is green.
import {
  useAlertChannels,
  useCreateAlertChannel,
  useDeleteAlertChannel,
} from '@/api/alert-channels';
import { useAlertPolicies, useCreateAlertPolicy, useDeleteAlertPolicy } from '@/api/alert-policies';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { createFileRoute } from '@tanstack/react-router';
import { BellRing, Plus, Trash2 } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/system/alerts')({
  component: AlertsPage,
});

function AlertsPage() {
  return (
    <>
      <PageHeader title='Alerts' subtitle='Channels and policies.' />
      <Tabs defaultValue='channels'>
        <TabsList>
          <TabsTrigger value='channels'>Channels</TabsTrigger>
          <TabsTrigger value='policies'>Policies</TabsTrigger>
        </TabsList>
        <TabsContent value='channels'>
          <ChannelsTab />
        </TabsContent>
        <TabsContent value='policies'>
          <PoliciesTab />
        </TabsContent>
      </Tabs>
    </>
  );
}

function ChannelsTab() {
  const { canMutate } = useAuth();
  const q = useAlertChannels();
  const del = useDeleteAlertChannel();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex justify-end'>
        {mayMutate && (
          <Button variant='primary' onClick={() => setCreateOpen(true)}>
            <Plus size={13} /> New channel
          </Button>
        )}
      </div>
      {q.isLoading ? (
        <Skeleton className='h-24' />
      ) : q.isError ? (
        <EmptyState
          icon={<BellRing size={28} />}
          title='Unable to load channels'
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>Retry</Button>}
        />
      ) : (q.data?.length ?? 0) === 0 ? (
        <EmptyState icon={<BellRing size={28} />} title='No alert channels yet' />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>Name</TableHeaderCell>
                <TableHeaderCell>Type</TableHeaderCell>
                <TableHeaderCell>Min severity</TableHeaderCell>
                <TableHeaderCell>Last delivery</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {q.data!.map((c) => (
                <TableRow key={c.metadata.name}>
                  <TableCell className='mono text-xs'>{c.metadata.name}</TableCell>
                  <TableCell>
                    <Badge>{c.spec.type}</Badge>
                  </TableCell>
                  <TableCell className='text-xs'>{c.spec.minSeverity ?? '—'}</TableCell>
                  <TableCell className='mono text-xs'>{c.status?.lastDeliveryAt ?? '—'}</TableCell>
                  <TableCell className='text-right'>
                    {mayMutate && (
                      <Button
                        size='sm'
                        variant='danger'
                        onClick={async () => {
                          try {
                            await del.mutateAsync(c.metadata.name);
                            toast.success('Deleted', c.metadata.name);
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
      <CreateChannelDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}

function CreateChannelDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateAlertChannel();
  const toast = useToast();
  const [name, setName] = useState('');
  const [type, setType] = useState<'email' | 'webhook' | 'ntfy'>('email');
  const [to, setTo] = useState('');
  const [webhookUrl, setWebhookUrl] = useState('');
  const [ntfyTopic, setNtfyTopic] = useState('');
  const [minSeverity, setMinSeverity] = useState<'info' | 'warning' | 'critical'>('warning');

  const submit = async () => {
    if (!name) {
      toast.error('Missing name');
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          type,
          minSeverity,
          email:
            type === 'email'
              ? {
                  to: to
                    .split(',')
                    .map((s) => s.trim())
                    .filter(Boolean),
                }
              : undefined,
          webhook: type === 'webhook' ? { url: webhookUrl } : undefined,
          ntfy: type === 'ntfy' ? { topic: ntfyTopic } : undefined,
        },
      });
      toast.success('Channel created', name);
      setName('');
      setTo('');
      setWebhookUrl('');
      setNtfyTopic('');
      onOpenChange(false);
    } catch (e) {
      toast.error('Create failed', (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New alert channel</DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label='Name' required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label='Type'>
            <Select value={type} onValueChange={(v) => setType(v as typeof type)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='email'>email</SelectItem>
                <SelectItem value='webhook'>webhook</SelectItem>
                <SelectItem value='ntfy'>ntfy</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label='Minimum severity'>
            <Select
              value={minSeverity}
              onValueChange={(v) => setMinSeverity(v as typeof minSeverity)}
            >
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='info'>info</SelectItem>
                <SelectItem value='warning'>warning</SelectItem>
                <SelectItem value='critical'>critical</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          {type === 'email' && (
            <FormField label='To' hint='Comma-separated emails'>
              <Input value={to} onChange={(e) => setTo(e.target.value)} />
            </FormField>
          )}
          {type === 'webhook' && (
            <FormField label='Webhook URL' required>
              <Input value={webhookUrl} onChange={(e) => setWebhookUrl(e.target.value)} />
            </FormField>
          )}
          {type === 'ntfy' && (
            <FormField label='Topic' required>
              <Input value={ntfyTopic} onChange={(e) => setNtfyTopic(e.target.value)} />
            </FormField>
          )}
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

function PoliciesTab() {
  const { canMutate } = useAuth();
  const q = useAlertPolicies();
  const del = useDeleteAlertPolicy();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <div className='flex flex-col gap-3'>
      <div className='flex justify-end'>
        {mayMutate && (
          <Button variant='primary' onClick={() => setCreateOpen(true)}>
            <Plus size={13} /> New policy
          </Button>
        )}
      </div>
      {q.isLoading ? (
        <Skeleton className='h-24' />
      ) : q.isError ? (
        <EmptyState
          icon={<BellRing size={28} />}
          title='Unable to load policies'
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>Retry</Button>}
        />
      ) : (q.data?.length ?? 0) === 0 ? (
        <EmptyState icon={<BellRing size={28} />} title='No alert policies yet' />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                <TableHeaderCell>Name</TableHeaderCell>
                <TableHeaderCell>Severity</TableHeaderCell>
                <TableHeaderCell>Condition</TableHeaderCell>
                <TableHeaderCell>Channels</TableHeaderCell>
                <TableHeaderCell>Last fired</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {q.data!.map((p) => (
                <TableRow key={p.metadata.name}>
                  <TableCell className='mono text-xs'>{p.metadata.name}</TableCell>
                  <TableCell>
                    <Badge>{p.spec.severity}</Badge>
                  </TableCell>
                  <TableCell className='mono text-xs'>
                    {p.spec.condition.query} {p.spec.condition.operator}{' '}
                    {p.spec.condition.threshold}
                  </TableCell>
                  <TableCell>
                    <div className='flex gap-1 flex-wrap'>
                      {p.spec.channels.map((c) => (
                        <Badge key={c}>{c}</Badge>
                      ))}
                    </div>
                  </TableCell>
                  <TableCell className='mono text-xs'>{p.status?.lastFired ?? '—'}</TableCell>
                  <TableCell className='text-right'>
                    {mayMutate && (
                      <Button
                        size='sm'
                        variant='danger'
                        onClick={async () => {
                          try {
                            await del.mutateAsync(p.metadata.name);
                            toast.success('Deleted', p.metadata.name);
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
      <CreatePolicyDialog open={createOpen} onOpenChange={setCreateOpen} />
    </div>
  );
}

function CreatePolicyDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateAlertPolicy();
  const channels = useAlertChannels();
  const toast = useToast();
  const [name, setName] = useState('');
  const [severity, setSeverity] = useState<'info' | 'warning' | 'critical'>('warning');
  const [query, setQuery] = useState('');
  const [operator, setOperator] = useState<'>' | '<' | '>=' | '<=' | '==' | '!='>('>');
  const [threshold, setThreshold] = useState('0');
  const [channel, setChannel] = useState('');

  const submit = async () => {
    if (!name || !query || !channel) {
      toast.error('Missing fields');
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          severity,
          condition: { query, operator, threshold: Number.parseFloat(threshold) || 0 },
          channels: [channel],
        },
      });
      toast.success('Policy created', name);
      setName('');
      setQuery('');
      setThreshold('0');
      onOpenChange(false);
    } catch (e) {
      toast.error('Create failed', (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>New alert policy</DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label='Name' required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label='Severity'>
            <Select value={severity} onValueChange={(v) => setSeverity(v as typeof severity)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='info'>info</SelectItem>
                <SelectItem value='warning'>warning</SelectItem>
                <SelectItem value='critical'>critical</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label='Metric query (PromQL)' required>
            <Input value={query} onChange={(e) => setQuery(e.target.value)} />
          </FormField>
          <div className='grid grid-cols-2 gap-3'>
            <FormField label='Operator'>
              <Select value={operator} onValueChange={(v) => setOperator(v as typeof operator)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  {['>', '<', '>=', '<=', '==', '!='].map((o) => (
                    <SelectItem key={o} value={o}>
                      {o}
                    </SelectItem>
                  ))}
                </SelectContent>
              </Select>
            </FormField>
            <FormField label='Threshold'>
              <Input
                type='number'
                value={threshold}
                onChange={(e) => setThreshold(e.target.value)}
              />
            </FormField>
          </div>
          <FormField label='Notify channel' required>
            <Select value={channel} onValueChange={setChannel}>
              <SelectTrigger>
                <SelectValue placeholder={channels.isLoading ? 'Loading…' : 'Select'} />
              </SelectTrigger>
              <SelectContent>
                {channels.data?.map((c) => (
                  <SelectItem key={c.metadata.name} value={c.metadata.name}>
                    {c.metadata.name}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
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
