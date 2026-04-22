import { useBonds, useCreateBond, useDeleteBond } from '@/api/bonds';
import {
  useCreateCustomDomain,
  useCustomDomains,
  useDeleteCustomDomain,
} from '@/api/custom-domains';
import {
  useCreateFirewallRule,
  useDeleteFirewallRule,
  useFirewallRules,
} from '@/api/firewall-rules';
import {
  useCreateHostInterface,
  useDeleteHostInterface,
  useHostInterfaces,
} from '@/api/host-interfaces';
import { useCreateIngress, useDeleteIngress, useIngresses } from '@/api/ingresses';
import { usePhysicalInterfaces } from '@/api/physical-interfaces';
import {
  useCreateRemoteAccessTunnel,
  useDeleteRemoteAccessTunnel,
  useRemoteAccessTunnels,
} from '@/api/remote-access-tunnels';
import {
  useCreateTrafficPolicy,
  useDeleteTrafficPolicy,
  useTrafficPolicies,
} from '@/api/traffic-policies';
import { useCreateVipPool, useDeleteVipPool, useVipPools } from '@/api/vip-pools';
import { useCreateVlan, useDeleteVlan, useVlans } from '@/api/vlans';
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { i18n } from '@/lib/i18n';
import { Trans } from '@lingui/react';
import { createFileRoute } from '@tanstack/react-router';
import { Network, Plus, Trash2 } from 'lucide-react';
import { type ReactNode, useState } from 'react';

export const Route = createFileRoute('/_auth/network')({
  component: NetworkPage,
});

function NetworkPage() {
  return (
    <>
      <PageHeader
        title={i18n._('Network')}
        subtitle={i18n._('Interfaces, routing, and firewall.')}
      />
      <Tabs defaultValue='interfaces'>
        <TabsList>
          <TabsTrigger value='interfaces'>
            <Trans id='Interfaces' />
          </TabsTrigger>
          <TabsTrigger value='routing'>
            <Trans id='Routing' />
          </TabsTrigger>
          <TabsTrigger value='security'>
            <Trans id='Security' />
          </TabsTrigger>
        </TabsList>
        <TabsContent value='interfaces' className='flex flex-col gap-5'>
          <PhysicalInterfacesSection />
          <BondsSection />
          <VlansSection />
          <HostInterfacesSection />
        </TabsContent>
        <TabsContent value='routing' className='flex flex-col gap-5'>
          <VipPoolsSection />
          <IngressesSection />
          <CustomDomainsSection />
          <RemoteAccessTunnelsSection />
        </TabsContent>
        <TabsContent value='security' className='flex flex-col gap-5'>
          <FirewallRulesSection />
          <TrafficPoliciesSection />
        </TabsContent>
      </Tabs>
    </>
  );
}

// Generic section wrapper -----------------------------------------------------

interface SectionProps<T> {
  title: string;
  subtitle?: string;
  items: T[] | undefined;
  isLoading: boolean;
  isError: boolean;
  error?: unknown;
  onRetry: () => void;
  empty: string;
  columns: string[];
  renderRow: (item: T) => ReactNode;
  actions?: ReactNode;
}

function Section<T>({
  title,
  subtitle,
  items,
  isLoading,
  isError,
  error,
  onRetry,
  empty,
  columns,
  renderRow,
  actions,
}: SectionProps<T>) {
  return (
    <section className='flex flex-col gap-2'>
      <div className='flex items-end justify-between'>
        <div>
          <h2 className='text-md font-semibold text-foreground'>{title}</h2>
          {subtitle && <p className='text-xs text-foreground-subtle'>{subtitle}</p>}
        </div>
        {actions}
      </div>
      {isLoading ? (
        <Skeleton className='h-16' />
      ) : isError ? (
        <EmptyState
          icon={<Network size={22} />}
          title={`${i18n._('Unable to load')} ${title.toLowerCase()}`}
          description={(error as Error)?.message ?? i18n._('Try again in a moment.')}
          action={<Button onClick={onRetry}>{i18n._('Retry')}</Button>}
        />
      ) : !items || items.length === 0 ? (
        <EmptyState icon={<Network size={22} />} title={empty} />
      ) : (
        <div className='border border-border rounded-md overflow-hidden'>
          <Table>
            <TableHead>
              <tr>
                {columns.map((c) => (
                  <TableHeaderCell key={c}>{c}</TableHeaderCell>
                ))}
              </tr>
            </TableHead>
            <TableBody>{items.map(renderRow)}</TableBody>
          </Table>
        </div>
      )}
    </section>
  );
}

function toneForPhase(phase: string | undefined) {
  if (!phase) return 'idle' as const;
  if (phase === 'Active' || phase === 'Applied' || phase === 'Issued' || phase === 'Connected')
    return 'ok' as const;
  if (phase === 'Failed' || phase === 'Disconnected' || phase === 'Expired') return 'err' as const;
  if (phase === 'Degraded' || phase === 'Suspended' || phase === 'Renewing') return 'warn' as const;
  return 'idle' as const;
}

// -- Physical Interfaces (read-only) -----------------------------------------
function PhysicalInterfacesSection() {
  const q = usePhysicalInterfaces();
  return (
    <Section
      title={i18n._('Physical interfaces')}
      subtitle={i18n._('Observed NICs. Read-only.')}
      items={q.data}
      isLoading={q.isLoading}
      isError={q.isError}
      error={q.error}
      onRetry={() => q.refetch()}
      empty={i18n._('No physical interfaces discovered.')}
      columns={[
        i18n._('Name'),
        i18n._('Link'),
        i18n._('Speed'),
        i18n._('MAC'),
        i18n._('Driver'),
        i18n._('Used by'),
      ]}
      renderRow={(p) => (
        <TableRow key={p.metadata.name}>
          <TableCell>
            <StatusDot tone={p.status?.link === 'up' ? 'ok' : 'err'} className='mr-2' />
            <span className='mono text-xs'>{p.metadata.name}</span>
          </TableCell>
          <TableCell className='text-xs'>{p.status?.link ?? '—'}</TableCell>
          <TableCell className='text-xs'>
            {p.status?.speedMbps ? `${p.status.speedMbps} Mbps` : '—'}
          </TableCell>
          <TableCell className='mono text-xs'>{p.status?.macAddress ?? '—'}</TableCell>
          <TableCell className='mono text-xs'>{p.status?.driver ?? '—'}</TableCell>
          <TableCell className='mono text-xs'>{p.status?.usedBy ?? '—'}</TableCell>
        </TableRow>
      )}
    />
  );
}

// -- Bonds --------------------------------------------------------------------
function BondsSection() {
  const { canMutate } = useAuth();
  const q = useBonds();
  const del = useDeleteBond();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <Section
        title={i18n._('Bonds')}
        items={q.data}
        isLoading={q.isLoading}
        isError={q.isError}
        error={q.error}
        onRetry={() => q.refetch()}
        empty={i18n._('No bonds yet.')}
        columns={[i18n._('Name'), i18n._('Mode'), i18n._('Members'), i18n._('Phase'), '']}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New bond' />
            </Button>
          ) : null
        }
        renderRow={(b) => (
          <TableRow key={b.metadata.name}>
            <TableCell>
              <StatusDot tone={toneForPhase(b.status?.phase)} className='mr-2' />
              <span className='mono text-xs'>{b.metadata.name}</span>
            </TableCell>
            <TableCell className='text-xs'>{b.spec.mode}</TableCell>
            <TableCell className='mono text-xs'>{b.spec.interfaces.join(', ')}</TableCell>
            <TableCell className='text-xs'>{b.status?.phase ?? 'Pending'}</TableCell>
            <TableCell className='text-right'>
              {mayMutate && (
                <Button
                  size='sm'
                  variant='danger'
                  onClick={async () => {
                    try {
                      await del.mutateAsync(b.metadata.name);
                      toast.success(i18n._('Bond deleted'), b.metadata.name);
                    } catch (e) {
                      toast.error(i18n._('Delete failed'), (e as Error).message);
                    }
                  }}
                >
                  <Trash2 size={12} />
                </Button>
              )}
            </TableCell>
          </TableRow>
        )}
      />
      <CreateBondDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateBondDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateBond();
  const toast = useToast();
  const [name, setName] = useState('');
  const [mode, setMode] = useState<'active-backup' | '802.3ad' | 'balance-alb'>('active-backup');
  const [interfaces, setInterfaces] = useState('');

  const submit = async () => {
    if (!name || !interfaces) {
      toast.error(i18n._('Missing fields'), 'Name and interfaces are required.');
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          mode,
          interfaces: interfaces
            .split(',')
            .map((s) => s.trim())
            .filter(Boolean),
        },
      });
      toast.success(i18n._('Bond created'), name);
      setName('');
      setInterfaces('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New bond' />
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} placeholder='bond0' />
          </FormField>
          <FormField label={i18n._('Mode')} required>
            <Select value={mode} onValueChange={(v) => setMode(v as typeof mode)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='active-backup'>active-backup</SelectItem>
                <SelectItem value='802.3ad'>802.3ad (LACP)</SelectItem>
                <SelectItem value='balance-alb'>balance-alb</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label={i18n._('Interfaces')} hint={i18n._('Comma-separated')} required>
            <Input
              value={interfaces}
              onChange={(e) => setInterfaces(e.target.value)}
              placeholder='eth0, eth1'
            />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -- VLANs --------------------------------------------------------------------
function VlansSection() {
  const { canMutate } = useAuth();
  const q = useVlans();
  const del = useDeleteVlan();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <Section
        title={i18n._('VLANs')}
        items={q.data}
        isLoading={q.isLoading}
        isError={q.isError}
        error={q.error}
        onRetry={() => q.refetch()}
        empty={i18n._('No VLANs configured.')}
        columns={[i18n._('Name'), i18n._('Parent'), i18n._('VLAN ID'), i18n._('MTU'), '']}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New VLAN' />
            </Button>
          ) : null
        }
        renderRow={(v) => (
          <TableRow key={v.metadata.name}>
            <TableCell className='mono text-xs'>{v.metadata.name}</TableCell>
            <TableCell className='mono text-xs'>{v.spec.parent}</TableCell>
            <TableCell className='mono text-xs'>{v.spec.vlanId}</TableCell>
            <TableCell className='mono text-xs'>{v.spec.mtu ?? '—'}</TableCell>
            <TableCell className='text-right'>
              {mayMutate && (
                <Button
                  size='sm'
                  variant='danger'
                  onClick={async () => {
                    try {
                      await del.mutateAsync(v.metadata.name);
                      toast.success(i18n._('VLAN deleted'), v.metadata.name);
                    } catch (e) {
                      toast.error(i18n._('Delete failed'), (e as Error).message);
                    }
                  }}
                >
                  <Trash2 size={12} />
                </Button>
              )}
            </TableCell>
          </TableRow>
        )}
      />
      <CreateVlanDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateVlanDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateVlan();
  const toast = useToast();
  const [name, setName] = useState('');
  const [parent, setParent] = useState('');
  const [vlanId, setVlanId] = useState('');

  const submit = async () => {
    const id = Number.parseInt(vlanId, 10);
    if (!name || !parent || !id) {
      toast.error(i18n._('Missing fields'), 'Name, parent, and VLAN ID are required.');
      return;
    }
    try {
      await create.mutateAsync({ metadata: { name }, spec: { parent, vlanId: id } });
      toast.success(i18n._('VLAN created'), name);
      setName('');
      setParent('');
      setVlanId('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New VLAN' />
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Parent interface')} required>
            <Input value={parent} onChange={(e) => setParent(e.target.value)} />
          </FormField>
          <FormField label={i18n._('VLAN ID')} required>
            <Input
              type='number'
              value={vlanId}
              onChange={(e) => setVlanId(e.target.value)}
              placeholder='10'
            />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -- Host Interfaces ----------------------------------------------------------
function HostInterfacesSection() {
  const { canMutate } = useAuth();
  const q = useHostInterfaces();
  const del = useDeleteHostInterface();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <Section
        title={i18n._('Host interfaces')}
        subtitle={i18n._('Logical layer-3 assignments and usage bindings.')}
        items={q.data}
        isLoading={q.isLoading}
        isError={q.isError}
        error={q.error}
        onRetry={() => q.refetch()}
        empty={i18n._('No host interfaces yet.')}
        columns={[
          i18n._('Name'),
          i18n._('Backing'),
          i18n._('Addresses'),
          i18n._('Usage'),
          i18n._('Link'),
          '',
        ]}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New host interface' />
            </Button>
          ) : null
        }
        renderRow={(h) => (
          <TableRow key={h.metadata.name}>
            <TableCell>
              <StatusDot tone={toneForPhase(h.status?.phase)} className='mr-2' />
              <span className='mono text-xs'>{h.metadata.name}</span>
            </TableCell>
            <TableCell className='mono text-xs'>{h.spec.backing}</TableCell>
            <TableCell className='mono text-xs'>
              {h.status?.effectiveAddresses?.join(', ') ??
                h.spec.addresses?.map((a) => a.cidr).join(', ') ??
                '—'}
            </TableCell>
            <TableCell>
              <div className='flex gap-1 flex-wrap'>
                {h.spec.usage.map((u) => (
                  <Badge key={u}>{u}</Badge>
                ))}
              </div>
            </TableCell>
            <TableCell className='text-xs'>{h.status?.link ?? '—'}</TableCell>
            <TableCell className='text-right'>
              {mayMutate && (
                <Button
                  size='sm'
                  variant='danger'
                  onClick={async () => {
                    try {
                      await del.mutateAsync(h.metadata.name);
                      toast.success(i18n._('Deleted'), h.metadata.name);
                    } catch (e) {
                      toast.error(i18n._('Delete failed'), (e as Error).message);
                    }
                  }}
                >
                  <Trash2 size={12} />
                </Button>
              )}
            </TableCell>
          </TableRow>
        )}
      />
      <CreateHostInterfaceDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateHostInterfaceDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateHostInterface();
  const toast = useToast();
  const [name, setName] = useState('');
  const [backing, setBacking] = useState('');
  const [cidr, setCidr] = useState('');
  const [usage, setUsage] = useState<
    'management' | 'storage' | 'cluster' | 'vmBridge' | 'appIngress'
  >('management');

  const submit = async () => {
    if (!name || !backing) {
      toast.error(i18n._('Missing fields'), 'Name and backing are required.');
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          backing,
          usage: [usage],
          addresses: cidr ? [{ cidr, type: 'static' }] : undefined,
        },
      });
      toast.success(i18n._('Host interface created'), name);
      setName('');
      setBacking('');
      setCidr('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New host interface' />
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField
            label={i18n._('Backing')}
            hint={i18n._('Physical, bond, or VLAN name')}
            required
          >
            <Input value={backing} onChange={(e) => setBacking(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Static CIDR')}>
            <Input
              value={cidr}
              onChange={(e) => setCidr(e.target.value)}
              placeholder='10.0.0.2/24'
            />
          </FormField>
          <FormField label={i18n._('Usage')} required>
            <Select value={usage} onValueChange={(v) => setUsage(v as typeof usage)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='management'>management</SelectItem>
                <SelectItem value='storage'>storage</SelectItem>
                <SelectItem value='cluster'>cluster</SelectItem>
                <SelectItem value='vmBridge'>vmBridge</SelectItem>
                <SelectItem value='appIngress'>appIngress</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -- VIP Pools ----------------------------------------------------------------
function VipPoolsSection() {
  const { canMutate } = useAuth();
  const q = useVipPools();
  const del = useDeleteVipPool();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <Section
        title={i18n._('VIP pools')}
        subtitle={i18n._('Floating address pools announced via ARP/BGP.')}
        items={q.data}
        isLoading={q.isLoading}
        isError={q.isError}
        error={q.error}
        onRetry={() => q.refetch()}
        empty={i18n._('No VIP pools yet.')}
        columns={[i18n._('Name'), i18n._('Range'), i18n._('Interface'), i18n._('Allocated'), '']}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New VIP pool' />
            </Button>
          ) : null
        }
        renderRow={(v) => (
          <TableRow key={v.metadata.name}>
            <TableCell>
              <StatusDot tone={toneForPhase(v.status?.phase)} className='mr-2' />
              <span className='mono text-xs'>{v.metadata.name}</span>
            </TableCell>
            <TableCell className='mono text-xs'>{v.spec.range}</TableCell>
            <TableCell className='mono text-xs'>{v.spec.interface}</TableCell>
            <TableCell className='mono text-xs'>
              {v.status?.allocated ?? 0} / {(v.status?.allocated ?? 0) + (v.status?.available ?? 0)}
            </TableCell>
            <TableCell className='text-right'>
              {mayMutate && (
                <Button
                  size='sm'
                  variant='danger'
                  onClick={async () => {
                    try {
                      await del.mutateAsync(v.metadata.name);
                      toast.success(i18n._('Deleted'), v.metadata.name);
                    } catch (e) {
                      toast.error(i18n._('Delete failed'), (e as Error).message);
                    }
                  }}
                >
                  <Trash2 size={12} />
                </Button>
              )}
            </TableCell>
          </TableRow>
        )}
      />
      <CreateVipPoolDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateVipPoolDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateVipPool();
  const toast = useToast();
  const [name, setName] = useState('');
  const [range, setRange] = useState('');
  const [iface, setIface] = useState('');

  const submit = async () => {
    if (!name || !range || !iface) {
      toast.error(i18n._('Missing fields'), 'Name, range, and interface are required.');
      return;
    }
    try {
      await create.mutateAsync({ metadata: { name }, spec: { range, interface: iface } });
      toast.success(i18n._('VIP pool created'), name);
      setName('');
      setRange('');
      setIface('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New VIP pool' />
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Range (CIDR)')} required>
            <Input
              value={range}
              onChange={(e) => setRange(e.target.value)}
              placeholder='10.0.0.240/28'
            />
          </FormField>
          <FormField label={i18n._('Interface')} required>
            <Input value={iface} onChange={(e) => setIface(e.target.value)} />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -- Ingresses ----------------------------------------------------------------
function IngressesSection() {
  const { canMutate } = useAuth();
  const q = useIngresses();
  const del = useDeleteIngress();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <Section
        title={i18n._('Ingresses')}
        items={q.data}
        isLoading={q.isLoading}
        isError={q.isError}
        error={q.error}
        onRetry={() => q.refetch()}
        empty={i18n._('No ingresses yet.')}
        columns={[i18n._('Name'), i18n._('Hostname'), i18n._('Rules'), i18n._('VIP'), '']}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New ingress' />
            </Button>
          ) : null
        }
        renderRow={(i) => (
          <TableRow key={i.metadata.name}>
            <TableCell>
              <StatusDot tone={toneForPhase(i.status?.phase)} className='mr-2' />
              <span className='mono text-xs'>{i.metadata.name}</span>
            </TableCell>
            <TableCell className='mono text-xs'>{i.spec.hostname}</TableCell>
            <TableCell className='mono text-xs'>{i.spec.rules.length}</TableCell>
            <TableCell className='mono text-xs'>{i.status?.vip ?? '—'}</TableCell>
            <TableCell className='text-right'>
              {mayMutate && (
                <Button
                  size='sm'
                  variant='danger'
                  onClick={async () => {
                    try {
                      await del.mutateAsync(i.metadata.name);
                      toast.success(i18n._('Deleted'), i.metadata.name);
                    } catch (e) {
                      toast.error(i18n._('Delete failed'), (e as Error).message);
                    }
                  }}
                >
                  <Trash2 size={12} />
                </Button>
              )}
            </TableCell>
          </TableRow>
        )}
      />
      <CreateIngressDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateIngressDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateIngress();
  const toast = useToast();
  const [name, setName] = useState('');
  const [hostname, setHostname] = useState('');
  const [backend, setBackend] = useState('');

  const submit = async () => {
    if (!name || !hostname || !backend) {
      toast.error(i18n._('Missing fields'), 'Name, hostname, and backend are required.');
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: { hostname, rules: [{ host: hostname, backend }] },
      });
      toast.success(i18n._('Ingress created'), name);
      setName('');
      setHostname('');
      setBackend('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New ingress' />
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Hostname')} required>
            <Input
              value={hostname}
              onChange={(e) => setHostname(e.target.value)}
              placeholder='photos.example.com'
            />
          </FormField>
          <FormField label={i18n._('Backend')} required>
            <Input value={backend} onChange={(e) => setBackend(e.target.value)} />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -- Custom Domains -----------------------------------------------------------
function CustomDomainsSection() {
  const { canMutate } = useAuth();
  const q = useCustomDomains();
  const del = useDeleteCustomDomain();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <Section
        title={i18n._('Custom domains')}
        items={q.data}
        isLoading={q.isLoading}
        isError={q.isError}
        error={q.error}
        onRetry={() => q.refetch()}
        empty={i18n._('No custom domains yet.')}
        columns={[i18n._('Name'), i18n._('Hostname'), i18n._('TLS'), i18n._('Cert status'), '']}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New domain' />
            </Button>
          ) : null
        }
        renderRow={(d) => (
          <TableRow key={d.metadata.name}>
            <TableCell>
              <StatusDot tone={toneForPhase(d.status?.phase)} className='mr-2' />
              <span className='mono text-xs'>{d.metadata.name}</span>
            </TableCell>
            <TableCell className='mono text-xs'>{d.spec.hostname}</TableCell>
            <TableCell className='text-xs'>{d.spec.tls.provider}</TableCell>
            <TableCell className='text-xs'>{d.status?.certificateStatus ?? '—'}</TableCell>
            <TableCell className='text-right'>
              {mayMutate && (
                <Button
                  size='sm'
                  variant='danger'
                  onClick={async () => {
                    try {
                      await del.mutateAsync(d.metadata.name);
                      toast.success(i18n._('Deleted'), d.metadata.name);
                    } catch (e) {
                      toast.error(i18n._('Delete failed'), (e as Error).message);
                    }
                  }}
                >
                  <Trash2 size={12} />
                </Button>
              )}
            </TableCell>
          </TableRow>
        )}
      />
      <CreateCustomDomainDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateCustomDomainDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateCustomDomain();
  const toast = useToast();
  const [name, setName] = useState('');
  const [hostname, setHostname] = useState('');
  const [targetKind, setTargetKind] = useState('App');
  const [targetName, setTargetName] = useState('');
  const [provider, setProvider] = useState<'letsencrypt' | 'internal' | 'upload'>('letsencrypt');

  const submit = async () => {
    if (!name || !hostname || !targetName) {
      toast.error(i18n._('Missing fields'));
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          hostname,
          target: { kind: targetKind, name: targetName },
          tls: { provider },
        },
      });
      toast.success(i18n._('Custom domain created'), name);
      setName('');
      setHostname('');
      setTargetName('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New custom domain' />
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Hostname')} required>
            <Input value={hostname} onChange={(e) => setHostname(e.target.value)} />
          </FormField>
          <div className='grid grid-cols-2 gap-3'>
            <FormField label={i18n._('Target kind')}>
              <Select value={targetKind} onValueChange={setTargetKind}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='App'>App</SelectItem>
                  <SelectItem value='Vm'>Vm</SelectItem>
                  <SelectItem value='Ingress'>Ingress</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            <FormField label={i18n._('Target name')} required>
              <Input value={targetName} onChange={(e) => setTargetName(e.target.value)} />
            </FormField>
          </div>
          <FormField label={i18n._('TLS provider')} required>
            <Select value={provider} onValueChange={(v) => setProvider(v as typeof provider)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='letsencrypt'>letsencrypt</SelectItem>
                <SelectItem value='internal'>internal</SelectItem>
                <SelectItem value='upload'>upload</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -- Remote Access Tunnels ----------------------------------------------------
function RemoteAccessTunnelsSection() {
  const { canMutate } = useAuth();
  const q = useRemoteAccessTunnels();
  const del = useDeleteRemoteAccessTunnel();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <Section
        title={i18n._('Remote access tunnels')}
        items={q.data}
        isLoading={q.isLoading}
        isError={q.isError}
        error={q.error}
        onRetry={() => q.refetch()}
        empty={i18n._('No remote tunnels yet.')}
        columns={[i18n._('Name'), i18n._('Type'), i18n._('Endpoint'), i18n._('Phase'), '']}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New tunnel' />
            </Button>
          ) : null
        }
        renderRow={(t) => (
          <TableRow key={t.metadata.name}>
            <TableCell>
              <StatusDot tone={toneForPhase(t.status?.phase)} className='mr-2' />
              <span className='mono text-xs'>{t.metadata.name}</span>
            </TableCell>
            <TableCell className='text-xs'>{t.spec.type}</TableCell>
            <TableCell className='mono text-xs'>
              {t.spec.endpoint.hostname}
              {t.spec.endpoint.port ? `:${t.spec.endpoint.port}` : ''}
            </TableCell>
            <TableCell className='text-xs'>{t.status?.phase ?? 'Pending'}</TableCell>
            <TableCell className='text-right'>
              {mayMutate && (
                <Button
                  size='sm'
                  variant='danger'
                  onClick={async () => {
                    try {
                      await del.mutateAsync(t.metadata.name);
                      toast.success(i18n._('Deleted'), t.metadata.name);
                    } catch (e) {
                      toast.error(i18n._('Delete failed'), (e as Error).message);
                    }
                  }}
                >
                  <Trash2 size={12} />
                </Button>
              )}
            </TableCell>
          </TableRow>
        )}
      />
      <CreateTunnelDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateTunnelDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateRemoteAccessTunnel();
  const toast = useToast();
  const [name, setName] = useState('');
  const [type, setType] = useState<'sdwan' | 'wireguard' | 'tailscale'>('wireguard');
  const [hostname, setHostname] = useState('');
  const [secretRef, setSecretRef] = useState('');

  const submit = async () => {
    if (!name || !hostname) {
      toast.error(i18n._('Missing fields'));
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          type,
          endpoint: { hostname },
          auth: { secretRef: secretRef || undefined },
        },
      });
      toast.success(i18n._('Tunnel created'), name);
      setName('');
      setHostname('');
      setSecretRef('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New remote access tunnel' />
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Type')}>
            <Select value={type} onValueChange={(v) => setType(v as typeof type)}>
              <SelectTrigger>
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value='wireguard'>WireGuard</SelectItem>
                <SelectItem value='tailscale'>Tailscale</SelectItem>
                <SelectItem value='sdwan'>SD-WAN</SelectItem>
              </SelectContent>
            </Select>
          </FormField>
          <FormField label={i18n._('Endpoint hostname')} required>
            <Input value={hostname} onChange={(e) => setHostname(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Secret ref')} hint={i18n._('Name of a stored secret')}>
            <Input value={secretRef} onChange={(e) => setSecretRef(e.target.value)} />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -- Firewall rules -----------------------------------------------------------
function FirewallRulesSection() {
  const { canMutate } = useAuth();
  const q = useFirewallRules();
  const del = useDeleteFirewallRule();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <Section
        title={i18n._('Firewall rules')}
        items={q.data}
        isLoading={q.isLoading}
        isError={q.isError}
        error={q.error}
        onRetry={() => q.refetch()}
        empty={i18n._('No firewall rules yet.')}
        columns={[
          i18n._('Name'),
          i18n._('Scope'),
          i18n._('Direction'),
          i18n._('Action'),
          i18n._('Priority'),
          '',
        ]}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New rule' />
            </Button>
          ) : null
        }
        renderRow={(r) => (
          <TableRow key={r.metadata.name}>
            <TableCell>
              <StatusDot tone={toneForPhase(r.status?.phase)} className='mr-2' />
              <span className='mono text-xs'>{r.metadata.name}</span>
            </TableCell>
            <TableCell className='text-xs'>{r.spec.scope}</TableCell>
            <TableCell className='text-xs'>{r.spec.direction}</TableCell>
            <TableCell>
              <Badge>{r.spec.action}</Badge>
            </TableCell>
            <TableCell className='mono text-xs'>{r.spec.priority ?? '—'}</TableCell>
            <TableCell className='text-right'>
              {mayMutate && (
                <Button
                  size='sm'
                  variant='danger'
                  onClick={async () => {
                    try {
                      await del.mutateAsync(r.metadata.name);
                      toast.success(i18n._('Deleted'), r.metadata.name);
                    } catch (e) {
                      toast.error(i18n._('Delete failed'), (e as Error).message);
                    }
                  }}
                >
                  <Trash2 size={12} />
                </Button>
              )}
            </TableCell>
          </TableRow>
        )}
      />
      <CreateFirewallRuleDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateFirewallRuleDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateFirewallRule();
  const toast = useToast();
  const [name, setName] = useState('');
  const [scope, setScope] = useState<'host' | 'pod'>('host');
  const [direction, setDirection] = useState<'inbound' | 'outbound'>('inbound');
  const [action, setAction] = useState<'allow' | 'deny' | 'reject' | 'log'>('allow');
  const [priority, setPriority] = useState('');
  const [sourceCidrs, setSourceCidrs] = useState('');
  const [destPorts, setDestPorts] = useState('');

  const submit = async () => {
    if (!name) {
      toast.error(i18n._('Missing name'));
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          scope,
          direction,
          action,
          priority: priority ? Number.parseInt(priority, 10) : undefined,
          source: sourceCidrs
            ? {
                cidrs: sourceCidrs
                  .split(',')
                  .map((s) => s.trim())
                  .filter(Boolean),
              }
            : undefined,
          destination: destPorts
            ? {
                ports: destPorts
                  .split(',')
                  .map((s) => Number.parseInt(s.trim(), 10))
                  .filter((n) => !Number.isNaN(n)),
              }
            : undefined,
        },
      });
      toast.success(i18n._('Rule created'), name);
      setName('');
      setPriority('');
      setSourceCidrs('');
      setDestPorts('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New firewall rule' />
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <div className='grid grid-cols-3 gap-3'>
            <FormField label={i18n._('Scope')}>
              <Select value={scope} onValueChange={(v) => setScope(v as typeof scope)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='host'>host</SelectItem>
                  <SelectItem value='pod'>pod</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            <FormField label={i18n._('Direction')}>
              <Select value={direction} onValueChange={(v) => setDirection(v as typeof direction)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='inbound'>inbound</SelectItem>
                  <SelectItem value='outbound'>outbound</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            <FormField label={i18n._('Action')}>
              <Select value={action} onValueChange={(v) => setAction(v as typeof action)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='allow'>allow</SelectItem>
                  <SelectItem value='deny'>deny</SelectItem>
                  <SelectItem value='reject'>reject</SelectItem>
                  <SelectItem value='log'>log</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
          </div>
          <FormField label={i18n._('Priority')}>
            <Input
              type='number'
              value={priority}
              onChange={(e) => setPriority(e.target.value)}
              placeholder='100'
            />
          </FormField>
          <FormField label={i18n._('Source CIDRs')} hint={i18n._('Comma-separated')}>
            <Input value={sourceCidrs} onChange={(e) => setSourceCidrs(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Destination ports')} hint={i18n._('Comma-separated')}>
            <Input value={destPorts} onChange={(e) => setDestPorts(e.target.value)} />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

// -- Traffic policies ---------------------------------------------------------
function TrafficPoliciesSection() {
  const { canMutate } = useAuth();
  const q = useTrafficPolicies();
  const del = useDeleteTrafficPolicy();
  const toast = useToast();
  const [createOpen, setCreateOpen] = useState(false);
  const mayMutate = canMutate();

  return (
    <>
      <Section
        title={i18n._('Traffic policies')}
        items={q.data}
        isLoading={q.isLoading}
        isError={q.isError}
        error={q.error}
        onRetry={() => q.refetch()}
        empty={i18n._('No traffic policies yet.')}
        columns={[i18n._('Name'), i18n._('Scope'), i18n._('Egress max'), i18n._('Ingress max'), '']}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> <Trans id='New policy' />
            </Button>
          ) : null
        }
        renderRow={(t) => (
          <TableRow key={t.metadata.name}>
            <TableCell>
              <StatusDot tone={toneForPhase(t.status?.phase)} className='mr-2' />
              <span className='mono text-xs'>{t.metadata.name}</span>
            </TableCell>
            <TableCell className='mono text-xs'>
              {t.spec.scope.kind}:{t.spec.scope.name}
            </TableCell>
            <TableCell className='mono text-xs'>{t.spec.limits?.egress?.max ?? '—'}</TableCell>
            <TableCell className='mono text-xs'>{t.spec.limits?.ingress?.max ?? '—'}</TableCell>
            <TableCell className='text-right'>
              {mayMutate && (
                <Button
                  size='sm'
                  variant='danger'
                  onClick={async () => {
                    try {
                      await del.mutateAsync(t.metadata.name);
                      toast.success(i18n._('Deleted'), t.metadata.name);
                    } catch (e) {
                      toast.error(i18n._('Delete failed'), (e as Error).message);
                    }
                  }}
                >
                  <Trash2 size={12} />
                </Button>
              )}
            </TableCell>
          </TableRow>
        )}
      />
      <CreateTrafficPolicyDialog open={createOpen} onOpenChange={setCreateOpen} />
    </>
  );
}

function CreateTrafficPolicyDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateTrafficPolicy();
  const toast = useToast();
  const [name, setName] = useState('');
  const [scopeKind, setScopeKind] = useState<
    'HostInterface' | 'Namespace' | 'App' | 'Vm' | 'ReplicationJob' | 'ObjectStore'
  >('HostInterface');
  const [scopeName, setScopeName] = useState('');
  const [egressMax, setEgressMax] = useState('');
  const [ingressMax, setIngressMax] = useState('');

  const submit = async () => {
    if (!name || !scopeName) {
      toast.error(i18n._('Missing fields'));
      return;
    }
    try {
      await create.mutateAsync({
        metadata: { name },
        spec: {
          scope: { kind: scopeKind, name: scopeName },
          limits: {
            egress: egressMax ? { max: egressMax } : undefined,
            ingress: ingressMax ? { max: ingressMax } : undefined,
          },
        },
      });
      toast.success(i18n._('Policy created'), name);
      setName('');
      setScopeName('');
      setEgressMax('');
      setIngressMax('');
      onOpenChange(false);
    } catch (e) {
      toast.error(i18n._('Create failed'), (e as Error).message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>
            <Trans id='New traffic policy' />
          </DialogTitle>
        </DialogHeader>
        <div className='flex flex-col gap-3'>
          <FormField label={i18n._('Name')} required>
            <Input value={name} onChange={(e) => setName(e.target.value)} />
          </FormField>
          <div className='grid grid-cols-2 gap-3'>
            <FormField label={i18n._('Scope kind')}>
              <Select value={scopeKind} onValueChange={(v) => setScopeKind(v as typeof scopeKind)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='HostInterface'>HostInterface</SelectItem>
                  <SelectItem value='Namespace'>Namespace</SelectItem>
                  <SelectItem value='App'>App</SelectItem>
                  <SelectItem value='Vm'>Vm</SelectItem>
                  <SelectItem value='ReplicationJob'>ReplicationJob</SelectItem>
                  <SelectItem value='ObjectStore'>ObjectStore</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            <FormField label={i18n._('Scope name')} required>
              <Input value={scopeName} onChange={(e) => setScopeName(e.target.value)} />
            </FormField>
          </div>
          <FormField label={i18n._('Egress max')} hint={i18n._('e.g. 100Mbps')}>
            <Input value={egressMax} onChange={(e) => setEgressMax(e.target.value)} />
          </FormField>
          <FormField label={i18n._('Ingress max')} hint={i18n._('e.g. 100Mbps')}>
            <Input value={ingressMax} onChange={(e) => setIngressMax(e.target.value)} />
          </FormField>
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' onClick={submit} disabled={create.isPending}>
            {create.isPending ? <Trans id='Creating…' /> : <Trans id='Create' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
