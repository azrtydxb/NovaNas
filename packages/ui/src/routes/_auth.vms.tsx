// TODO(i18n-wave-12): strings on this page are still raw English. Migrate to <Trans>/i18n._() once wave 12 is green.
import { useDatasets } from '@/api/datasets';
import { useGpuDevices } from '@/api/gpu-devices';
import { useIsoLibraries } from '@/api/iso-libraries';
import { type VmCreateBody, useCreateVm, useDeleteVm, useVmAction, useVms } from '@/api/vms';
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
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { VncConsole } from '@/components/vm/vnc-console';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { maybeTrackJobFromResponse } from '@/stores/jobs';
import type { Vm, VmDisk, VmGpuPassthroughEntry, VmOsType, VmSpec } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { Pause, Play, Plus, RotateCw, Server, Square, Trash2, X } from 'lucide-react';
import { useState } from 'react';

export const Route = createFileRoute('/_auth/vms')({
  component: VmsPage,
});

function VmsPage() {
  const { canMutate } = useAuth();
  const vms = useVms();
  const [createOpen, setCreateOpen] = useState(false);
  const [selected, setSelected] = useState<Vm | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<Vm | null>(null);
  const mayMutate = canMutate();

  return (
    <>
      <PageHeader
        title='Virtual Machines'
        subtitle='KubeVirt-backed VMs with VNC console, ISO library and GPU passthrough.'
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={() => setCreateOpen(true)}>
              <Plus size={13} /> New VM
            </Button>
          ) : null
        }
      />

      {vms.isLoading ? (
        <div className='flex flex-col gap-2'>
          {[0, 1, 2].map((i) => (
            <Skeleton key={i} className='h-9' />
          ))}
        </div>
      ) : vms.isError ? (
        <EmptyState
          icon={<Server size={28} />}
          title='Unable to load VMs'
          description={(vms.error as Error)?.message ?? 'Try again in a moment.'}
          action={<Button onClick={() => vms.refetch()}>Retry</Button>}
        />
      ) : (vms.data?.length ?? 0) === 0 ? (
        <EmptyState
          icon={<Server size={28} />}
          title='No VMs yet'
          description='Launch a VM from an ISO or existing dataset.'
          action={
            mayMutate ? (
              <Button variant='primary' onClick={() => setCreateOpen(true)}>
                <Plus size={13} /> New VM
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
                <TableHeaderCell>OS</TableHeaderCell>
                <TableHeaderCell>CPU / Mem</TableHeaderCell>
                <TableHeaderCell>State</TableHeaderCell>
                <TableHeaderCell>IP</TableHeaderCell>
                <TableHeaderCell className='text-right'>Actions</TableHeaderCell>
              </tr>
            </TableHead>
            <TableBody>
              {vms.data!.map((vm) => (
                <VmRow
                  key={vm.metadata.name}
                  vm={vm}
                  mayMutate={mayMutate}
                  onSelect={() => setSelected(vm)}
                  onDelete={() => setDeleteTarget(vm)}
                />
              ))}
            </TableBody>
          </Table>
        </div>
      )}

      <CreateVmDialog open={createOpen} onOpenChange={setCreateOpen} />
      <DeleteVmDialog vm={deleteTarget} onOpenChange={(o) => !o && setDeleteTarget(null)} />
      <VmDetailDrawer vm={selected} onClose={() => setSelected(null)} />
    </>
  );
}

function VmRow({
  vm,
  mayMutate,
  onSelect,
  onDelete,
}: {
  vm: Vm;
  mayMutate: boolean;
  onSelect: () => void;
  onDelete: () => void;
}) {
  const phase = vm.status?.phase ?? 'Pending';
  const tone =
    phase === 'Running' ? 'ok' : phase === 'Failed' ? 'err' : phase === 'Paused' ? 'warn' : 'idle';
  const action = useVmAction(vm.metadata.name);
  const toast = useToast();
  const run = async (a: 'start' | 'stop' | 'reset' | 'pause' | 'resume') => {
    try {
      await action.mutateAsync(a);
      toast.success(`${a} requested`, vm.metadata.name);
    } catch (err) {
      toast.error(`${a} failed`, (err as Error)?.message);
    }
  };
  return (
    <TableRow className='cursor-pointer' onClick={onSelect}>
      <TableCell>
        <StatusDot tone={tone} className='mr-2' />
        <span className='text-foreground font-medium'>{vm.metadata.name}</span>
      </TableCell>
      <TableCell className='mono text-xs'>
        {vm.spec.os.type}
        {vm.spec.os.variant ? ` / ${vm.spec.os.variant}` : ''}
      </TableCell>
      <TableCell className='mono text-xs'>
        {vm.spec.resources.cpu} vCPU · {vm.spec.resources.memoryMiB} MiB
      </TableCell>
      <TableCell>
        <Badge>{phase}</Badge>
      </TableCell>
      <TableCell className='mono text-xs'>{vm.status?.ip ?? '—'}</TableCell>
      <TableCell className='text-right' onClick={(e) => e.stopPropagation()}>
        {mayMutate && (
          <div className='flex justify-end gap-1'>
            {phase === 'Running' ? (
              <>
                <Button size='sm' variant='ghost' title='Pause' onClick={() => run('pause')}>
                  <Pause size={11} />
                </Button>
                <Button size='sm' variant='ghost' title='Reset' onClick={() => run('reset')}>
                  <RotateCw size={11} />
                </Button>
                <Button size='sm' variant='ghost' title='Stop' onClick={() => run('stop')}>
                  <Square size={11} />
                </Button>
              </>
            ) : phase === 'Paused' ? (
              <Button size='sm' variant='ghost' title='Resume' onClick={() => run('resume')}>
                <Play size={11} />
              </Button>
            ) : (
              <Button size='sm' variant='ghost' title='Start' onClick={() => run('start')}>
                <Play size={11} />
              </Button>
            )}
            <Button size='sm' variant='danger' title='Delete' onClick={onDelete}>
              <Trash2 size={11} />
            </Button>
          </div>
        )}
      </TableCell>
    </TableRow>
  );
}

// -----------------------------------------------------------------------------
interface VmForm {
  name: string;
  osType: VmOsType;
  osVariant: string;
  cpu: number;
  memoryMiB: number;
  disks: VmDisk[];
  networkType: 'bridge' | 'pod' | 'masquerade';
  bridge: string;
  graphicsEnabled: boolean;
  graphicsType: 'spice' | 'vnc';
  /** Names of GpuDevice resources the user has selected for passthrough. */
  gpuDeviceNames: string[];
}

const defaultVmForm: VmForm = {
  name: '',
  osType: 'linux',
  osVariant: '',
  cpu: 2,
  memoryMiB: 2048,
  disks: [],
  networkType: 'masquerade',
  bridge: '',
  graphicsEnabled: true,
  graphicsType: 'spice',
  gpuDeviceNames: [],
};

function CreateVmDialog({
  open,
  onOpenChange,
}: {
  open: boolean;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateVm();
  const datasets = useDatasets();
  const isos = useIsoLibraries();
  // Only fetch GpuDevices once the dialog is actually open, to avoid making
  // an API call every time the VMs page mounts.
  const gpuDevices = useGpuDevices({ enabled: open });
  const toast = useToast();
  const [form, setForm] = useState<VmForm>(defaultVmForm);

  const reset = () => setForm(defaultVmForm);

  const submit = async () => {
    if (!form.name) {
      toast.error('Name required');
      return;
    }
    // Build vm.spec.gpu.passthrough from the selected GpuDevice resources.
    const gpuEntries: VmGpuPassthroughEntry[] = (gpuDevices.data ?? [])
      .filter((g) => form.gpuDeviceNames.includes(g.metadata.name))
      .map((g) => {
        const entry: VmGpuPassthroughEntry = {
          device: g.status?.deviceId ?? g.metadata.name,
        };
        if (g.status?.vendor) entry.vendor = g.status.vendor;
        if (g.status?.model) entry.deviceName = g.status.model;
        return entry;
      });
    const spec: VmSpec = {
      os: {
        type: form.osType,
        variant: form.osVariant || undefined,
      },
      resources: { cpu: form.cpu, memoryMiB: form.memoryMiB },
      disks: form.disks.length ? form.disks : undefined,
      network: [
        {
          type: form.networkType,
          bridge: form.networkType === 'bridge' ? form.bridge || undefined : undefined,
        },
      ],
      graphics: { enabled: form.graphicsEnabled, type: form.graphicsType },
      ...(gpuEntries.length ? { gpu: { passthrough: gpuEntries } } : {}),
    };
    const body: VmCreateBody = { metadata: { name: form.name }, spec };
    try {
      const resp = await create.mutateAsync(body);
      maybeTrackJobFromResponse(resp, `Create VM ${form.name}`);
      toast.success('VM created', form.name);
      reset();
      onOpenChange(false);
    } catch (err) {
      toast.error('Failed to create VM', (err as Error)?.message);
    }
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-lg'>
        <DialogHeader>
          <DialogTitle>New VM</DialogTitle>
          <DialogDescription>Configure OS, resources, disks and network.</DialogDescription>
        </DialogHeader>

        <Tabs defaultValue='os'>
          <TabsList>
            <TabsTrigger value='os'>OS</TabsTrigger>
            <TabsTrigger value='resources'>Resources</TabsTrigger>
            <TabsTrigger value='disks'>Disks</TabsTrigger>
            <TabsTrigger value='network'>Network</TabsTrigger>
            <TabsTrigger value='graphics'>Graphics</TabsTrigger>
          </TabsList>

          <TabsContent value='os' className='pt-3 flex flex-col gap-3'>
            <FormField label='Name' required>
              <Input
                value={form.name}
                onChange={(e) => setForm({ ...form, name: e.target.value })}
                placeholder='vm-01'
              />
            </FormField>
            <FormField label='OS type'>
              <Select
                value={form.osType}
                onValueChange={(v) => setForm({ ...form, osType: v as VmOsType })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='linux'>linux</SelectItem>
                  <SelectItem value='windows'>windows</SelectItem>
                  <SelectItem value='other'>other</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            <FormField label='Variant' hint='e.g. ubuntu2204, windows11'>
              <Input
                value={form.osVariant}
                onChange={(e) => setForm({ ...form, osVariant: e.target.value })}
              />
            </FormField>
          </TabsContent>

          <TabsContent value='resources' className='pt-3 flex flex-col gap-3'>
            <FormField label='vCPU'>
              <Input
                type='number'
                min={1}
                value={form.cpu}
                onChange={(e) => setForm({ ...form, cpu: Number(e.target.value) })}
              />
            </FormField>
            <FormField label='Memory (MiB)'>
              <Input
                type='number'
                min={256}
                value={form.memoryMiB}
                onChange={(e) => setForm({ ...form, memoryMiB: Number(e.target.value) })}
              />
            </FormField>
          </TabsContent>

          <TabsContent value='disks' className='pt-3 flex flex-col gap-2'>
            {form.disks.length === 0 && (
              <div className='text-xs text-foreground-subtle'>No disks added yet.</div>
            )}
            <ul className='flex flex-col gap-1'>
              {form.disks.map((d, i) => (
                <li
                  key={i}
                  className='flex items-center justify-between gap-2 border border-border rounded-sm p-1 text-xs mono'
                >
                  <span>
                    {d.name} · {d.source.type}
                    {d.source.type === 'dataset' && ` → ${d.source.dataset}`}
                    {d.source.type === 'iso' && ` → ${d.source.isoLibrary}`}
                  </span>
                  <Button
                    size='sm'
                    variant='ghost'
                    onClick={() =>
                      setForm({ ...form, disks: form.disks.filter((_, j) => j !== i) })
                    }
                  >
                    <X size={11} />
                  </Button>
                </li>
              ))}
            </ul>
            <AddDiskRow
              datasets={datasets.data ?? []}
              isos={isos.data ?? []}
              onAdd={(disk) => setForm({ ...form, disks: [...form.disks, disk] })}
            />
          </TabsContent>

          <TabsContent value='network' className='pt-3 flex flex-col gap-3'>
            <FormField label='Type'>
              <Select
                value={form.networkType}
                onValueChange={(v) => setForm({ ...form, networkType: v as VmForm['networkType'] })}
              >
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='masquerade'>masquerade (NAT)</SelectItem>
                  <SelectItem value='bridge'>bridge</SelectItem>
                  <SelectItem value='pod'>pod</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
            {form.networkType === 'bridge' && (
              <FormField label='Bridge interface'>
                <Input
                  value={form.bridge}
                  onChange={(e) => setForm({ ...form, bridge: e.target.value })}
                  placeholder='br0'
                />
              </FormField>
            )}
          </TabsContent>

          <TabsContent value='graphics' className='pt-3 flex flex-col gap-3'>
            <div className='flex items-center gap-2 text-sm'>
              <Checkbox
                checked={form.graphicsEnabled}
                onCheckedChange={(v) => setForm({ ...form, graphicsEnabled: !!v })}
              />
              Enable graphics console
            </div>
            {form.graphicsEnabled && (
              <FormField label='Protocol'>
                <Select
                  value={form.graphicsType}
                  onValueChange={(v) => setForm({ ...form, graphicsType: v as 'spice' | 'vnc' })}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='spice'>SPICE</SelectItem>
                    <SelectItem value='vnc'>VNC</SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
            )}

            <div className='pt-2 border-t border-border'>
              <div className='text-xs uppercase tracking-wider text-foreground-subtle mb-2'>
                GPU passthrough
              </div>
              {gpuDevices.isLoading ? (
                <Skeleton className='h-9' />
              ) : gpuDevices.isError || (gpuDevices.data?.length ?? 0) === 0 ? (
                <div className='text-xs text-foreground-subtle border border-border rounded-sm p-3'>
                  No GPUs detected.
                </div>
              ) : (
                <ul className='flex flex-col gap-1'>
                  {gpuDevices.data!.map((g) => {
                    const name = g.metadata.name;
                    const selected = form.gpuDeviceNames.includes(name);
                    const st = g.status ?? {};
                    const assigned = st.assignedTo?.name;
                    return (
                      <li
                        key={name}
                        className='flex items-center gap-2 border border-border rounded-sm p-2 text-xs'
                      >
                        <Checkbox
                          checked={selected}
                          onCheckedChange={(v) => {
                            const next = v
                              ? [...form.gpuDeviceNames, name]
                              : form.gpuDeviceNames.filter((n) => n !== name);
                            setForm({ ...form, gpuDeviceNames: next });
                          }}
                        />
                        <div className='flex-1'>
                          <div className='mono text-foreground'>{name}</div>
                          <div className='text-foreground-subtle mono'>
                            {st.vendor ?? '—'} {st.model ?? ''}{' '}
                            {st.pciAddress ? `· ${st.pciAddress}` : ''}
                          </div>
                        </div>
                        {assigned && <Badge>in use by {assigned}</Badge>}
                      </li>
                    );
                  })}
                </ul>
              )}
            </div>
          </TabsContent>
        </Tabs>

        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button variant='primary' disabled={!form.name || create.isPending} onClick={submit}>
            {create.isPending ? 'Creating…' : 'Create VM'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function AddDiskRow({
  datasets,
  isos,
  onAdd,
}: {
  datasets: ReturnType<typeof useDatasets>['data'] extends (infer U)[] | undefined ? U[] : never;
  isos: ReturnType<typeof useIsoLibraries>['data'] extends (infer U)[] | undefined ? U[] : never;
  onAdd: (d: VmDisk) => void;
}) {
  const [name, setName] = useState('');
  const [sourceType, setSourceType] = useState<'dataset' | 'iso'>('dataset');
  const [sourceValue, setSourceValue] = useState('');
  const add = () => {
    if (!name || !sourceValue) return;
    const disk: VmDisk =
      sourceType === 'dataset'
        ? { name, source: { type: 'dataset', dataset: sourceValue } }
        : { name, source: { type: 'iso', isoLibrary: sourceValue } };
    onAdd(disk);
    setName('');
    setSourceValue('');
  };
  return (
    <div className='grid grid-cols-4 gap-2 items-end'>
      <FormField label='Name'>
        <Input value={name} onChange={(e) => setName(e.target.value)} placeholder='root' />
      </FormField>
      <FormField label='Source'>
        <Select value={sourceType} onValueChange={(v) => setSourceType(v as 'dataset' | 'iso')}>
          <SelectTrigger>
            <SelectValue />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value='dataset'>Dataset</SelectItem>
            <SelectItem value='iso'>ISO</SelectItem>
          </SelectContent>
        </Select>
      </FormField>
      <FormField label={sourceType === 'dataset' ? 'Dataset' : 'ISO Library'}>
        <Select value={sourceValue} onValueChange={setSourceValue}>
          <SelectTrigger>
            <SelectValue placeholder='select' />
          </SelectTrigger>
          <SelectContent>
            {sourceType === 'dataset'
              ? datasets.map((d) => (
                  <SelectItem key={d.metadata.name} value={d.metadata.name}>
                    {d.metadata.name}
                  </SelectItem>
                ))
              : isos.map((i) => (
                  <SelectItem key={i.metadata.name} value={i.metadata.name}>
                    {i.metadata.name}
                  </SelectItem>
                ))}
          </SelectContent>
        </Select>
      </FormField>
      <Button variant='ghost' size='sm' onClick={add} disabled={!name || !sourceValue}>
        Add disk
      </Button>
    </div>
  );
}

function DeleteVmDialog({
  vm,
  onOpenChange,
}: {
  vm: Vm | null;
  onOpenChange: (v: boolean) => void;
}) {
  const del = useDeleteVm();
  const toast = useToast();
  const [deleteDisks, setDeleteDisks] = useState(false);
  return (
    <Dialog open={!!vm} onOpenChange={onOpenChange}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>Delete VM?</DialogTitle>
          <DialogDescription>
            VM <span className='mono text-foreground'>{vm?.metadata.name}</span> will be removed.
          </DialogDescription>
        </DialogHeader>
        <div className='flex items-center gap-2 text-sm'>
          <Checkbox checked={deleteDisks} onCheckedChange={(v) => setDeleteDisks(!!v)} />
          Also delete disks
        </div>
        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            Cancel
          </Button>
          <Button
            variant='danger'
            disabled={del.isPending}
            onClick={async () => {
              if (!vm) return;
              try {
                await del.mutateAsync({ name: vm.metadata.name, deleteDisks });
                toast.success('VM deleted', vm.metadata.name);
                onOpenChange(false);
              } catch (err) {
                toast.error('Failed to delete VM', (err as Error)?.message);
              }
            }}
          >
            {del.isPending ? 'Deleting…' : 'Delete'}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

function VmDetailDrawer({ vm, onClose }: { vm: Vm | null; onClose: () => void }) {
  return (
    <Dialog open={!!vm} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle>{vm?.metadata.name}</DialogTitle>
          <DialogDescription>
            {vm?.spec.os.type}
            {vm?.spec.os.variant ? ` / ${vm.spec.os.variant}` : ''} · {vm?.spec.resources.cpu} vCPU
            · {vm?.spec.resources.memoryMiB} MiB
          </DialogDescription>
        </DialogHeader>
        <div className='flex flex-col gap-3 text-xs'>
          <div>
            <div className='text-foreground-subtle uppercase tracking-wider mb-1'>Status</div>
            <div className='mono'>Phase: {vm?.status?.phase ?? 'Pending'}</div>
            {vm?.status?.ip && <div className='mono'>IP: {vm.status.ip}</div>}
          </div>
          <div>
            <div className='text-foreground-subtle uppercase tracking-wider mb-1'>Disks</div>
            <ul className='flex flex-col gap-0.5 mono'>
              {vm?.spec.disks?.map((d, i) => (
                <li key={i}>
                  {d.name} · {d.source.type}
                </li>
              ))}
              {(vm?.spec.disks?.length ?? 0) === 0 && (
                <li className='text-foreground-subtle'>No disks.</li>
              )}
            </ul>
          </div>
          <div>
            <div className='text-foreground-subtle uppercase tracking-wider mb-1'>Console</div>
            {vm ? <VncConsole vm={vm} /> : null}
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
