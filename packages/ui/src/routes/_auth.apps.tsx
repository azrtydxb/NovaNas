import {
  type AppInstanceCreateBody,
  useAppInstanceAction,
  useAppInstances,
  useCreateAppInstance,
  useDeleteAppInstance,
} from '@/api/app-instances';
import { useAppsAvailable } from '@/api/apps-available';
import { useDatasets } from '@/api/datasets';
import { SchemaForm } from '@/components/apps/schema-form';
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
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { i18n } from '@/lib/i18n';
import { maybeTrackJobFromResponse } from '@/stores/jobs';
import { Trans } from '@lingui/react';
import type { App, AppInstance, ExposureMode } from '@novanas/schemas';
import { createFileRoute } from '@tanstack/react-router';
import { AppWindow, Download, RefreshCw, Square, Trash2, X } from 'lucide-react';
import { useEffect, useMemo, useState } from 'react';

export const Route = createFileRoute('/_auth/apps')({
  component: AppsPage,
});

function AppsPage() {
  const { canMutate } = useAuth();
  const apps = useAppsAvailable();
  const instances = useAppInstances();
  const [category, setCategory] = useState<string | null>(null);
  const [search, setSearch] = useState('');
  const [installTarget, setInstallTarget] = useState<App | null>(null);
  const mayMutate = canMutate();

  const categories = useMemo(() => {
    const set = new Set<string>();
    for (const a of apps.data ?? []) {
      if (a.spec.category) set.add(a.spec.category);
    }
    return Array.from(set).sort();
  }, [apps.data]);

  const filtered = (apps.data ?? []).filter((a) => {
    if (category && a.spec.category !== category) return false;
    if (search) {
      const hay =
        `${a.metadata.name} ${a.spec.displayName} ${a.spec.description ?? ''}`.toLowerCase();
      if (!hay.includes(search.toLowerCase())) return false;
    }
    return true;
  });

  return (
    <>
      <PageHeader
        title={i18n._('Apps')}
        subtitle={i18n._('Curated catalog + installed apps running on k3s.')}
      />

      <div className='flex flex-col gap-3 mb-4'>
        <div className='flex gap-2 items-center flex-wrap'>
          <Input
            placeholder={i18n._('Search catalog…')}
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className='max-w-xs'
          />
          <Button
            variant={category === null ? 'primary' : 'ghost'}
            size='sm'
            onClick={() => setCategory(null)}
          >
            <Trans id='All' />
          </Button>
          {categories.map((c) => (
            <Button
              key={c}
              variant={category === c ? 'primary' : 'ghost'}
              size='sm'
              onClick={() => setCategory(c)}
            >
              {c}
            </Button>
          ))}
        </div>

        {apps.isLoading ? (
          <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3'>
            {[0, 1, 2, 3, 4, 5].map((i) => (
              <Skeleton key={i} className='h-24' />
            ))}
          </div>
        ) : apps.isError ? (
          <EmptyState
            icon={<AppWindow size={28} />}
            title={i18n._('Unable to load catalog')}
            description={(apps.error as Error)?.message ?? i18n._('Try again in a moment.')}
            action={<Button onClick={() => apps.refetch()}>{i18n._('Retry')}</Button>}
          />
        ) : filtered.length === 0 ? (
          <EmptyState
            icon={<AppWindow size={28} />}
            title={i18n._('No apps match')}
            description={i18n._('Try a different category or search term.')}
          />
        ) : (
          <div className='grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-3'>
            {filtered.map((a) => (
              <div
                key={a.metadata.name}
                className='border border-border rounded-md p-3 flex flex-col gap-2 bg-panel/60'
              >
                <div className='flex items-start gap-2'>
                  {a.spec.icon ? (
                    <img
                      src={a.spec.icon}
                      alt=''
                      className='h-8 w-8 rounded-sm'
                      onError={(e) => {
                        (e.currentTarget as HTMLImageElement).style.display = 'none';
                      }}
                    />
                  ) : (
                    <div className='h-8 w-8 rounded-sm bg-surface border border-border flex items-center justify-center'>
                      <AppWindow size={14} />
                    </div>
                  )}
                  <div className='flex-1'>
                    <div className='text-sm font-medium'>{a.spec.displayName}</div>
                    <div className='text-xs text-foreground-subtle mono'>
                      v{a.spec.version}
                      {a.spec.category && <> · {a.spec.category}</>}
                    </div>
                  </div>
                </div>
                {a.spec.description && (
                  <p className='text-xs text-foreground-muted line-clamp-2'>{a.spec.description}</p>
                )}
                <div className='flex justify-end'>
                  {mayMutate && (
                    <Button size='sm' variant='primary' onClick={() => setInstallTarget(a)}>
                      <Download size={11} /> <Trans id='Install' />
                    </Button>
                  )}
                </div>
              </div>
            ))}
          </div>
        )}
      </div>

      <div className='mt-6'>
        <h2 className='text-md font-semibold mb-2'>
          <Trans id='Installed apps' />
        </h2>
        {instances.isLoading ? (
          <Skeleton className='h-9' />
        ) : instances.isError ? (
          <div className='text-xs text-danger'>
            <Trans id='Failed to load installed apps.' />
          </div>
        ) : (instances.data?.length ?? 0) === 0 ? (
          <div className='text-sm text-foreground-subtle'>
            <Trans id='No apps installed yet.' />
          </div>
        ) : (
          <div className='border border-border rounded-md overflow-hidden'>
            <Table>
              <TableHead>
                <tr>
                  <TableHeaderCell>
                    <Trans id='Name' />
                  </TableHeaderCell>
                  <TableHeaderCell>
                    <Trans id='App' />
                  </TableHeaderCell>
                  <TableHeaderCell>
                    <Trans id='Version' />
                  </TableHeaderCell>
                  <TableHeaderCell>
                    <Trans id='State' />
                  </TableHeaderCell>
                  <TableHeaderCell className='text-right'>
                    <Trans id='Actions' />
                  </TableHeaderCell>
                </tr>
              </TableHead>
              <TableBody>
                {instances.data!.map((inst) => (
                  <InstanceRow key={inst.metadata.name} inst={inst} mayMutate={mayMutate} />
                ))}
              </TableBody>
            </Table>
          </div>
        )}
      </div>

      <InstallAppDialog app={installTarget} onOpenChange={(v) => !v && setInstallTarget(null)} />
    </>
  );
}

function InstanceRow({ inst, mayMutate }: { inst: AppInstance; mayMutate: boolean }) {
  const phase = inst.status?.phase ?? 'Pending';
  const tone =
    phase === 'Running'
      ? 'ok'
      : phase === 'Failed'
        ? 'err'
        : phase === 'Updating'
          ? 'warn'
          : 'idle';
  const action = useAppInstanceAction(inst.metadata.name);
  const del = useDeleteAppInstance();
  const toast = useToast();
  const [deleteOpen, setDeleteOpen] = useState(false);

  return (
    <>
      <TableRow>
        <TableCell>
          <StatusDot tone={tone} className='mr-2' />
          <span className='text-foreground font-medium'>{inst.metadata.name}</span>
        </TableCell>
        <TableCell className='mono text-xs'>{inst.spec.app}</TableCell>
        <TableCell className='mono text-xs'>{inst.spec.version}</TableCell>
        <TableCell>
          <Badge>{phase}</Badge>
        </TableCell>
        <TableCell className='text-right'>
          {mayMutate && (
            <div className='flex justify-end gap-1'>
              {phase !== 'Stopped' ? (
                <Button
                  size='sm'
                  variant='ghost'
                  title={i18n._('Stop')}
                  disabled={action.isPending}
                  onClick={async () => {
                    try {
                      await action.mutateAsync('stop');
                      toast.success(i18n._('Stop requested'), inst.metadata.name);
                    } catch (err) {
                      toast.error(i18n._('Stop failed'), (err as Error)?.message);
                    }
                  }}
                >
                  <Square size={11} />
                </Button>
              ) : (
                <Button
                  size='sm'
                  variant='ghost'
                  title={i18n._('Start')}
                  disabled={action.isPending}
                  onClick={async () => {
                    try {
                      await action.mutateAsync('start');
                    } catch (err) {
                      toast.error(i18n._('Start failed'), (err as Error)?.message);
                    }
                  }}
                >
                  ▶
                </Button>
              )}
              <Button
                size='sm'
                variant='ghost'
                title={i18n._('Update')}
                disabled={action.isPending}
                onClick={async () => {
                  try {
                    await action.mutateAsync('update');
                    toast.success(i18n._('Update requested'), inst.metadata.name);
                  } catch (err) {
                    toast.error(i18n._('Update failed'), (err as Error)?.message);
                  }
                }}
              >
                <RefreshCw size={11} />
              </Button>
              <Button
                size='sm'
                variant='danger'
                title={i18n._('Uninstall')}
                onClick={() => setDeleteOpen(true)}
              >
                <Trash2 size={11} />
              </Button>
            </div>
          )}
        </TableCell>
      </TableRow>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>
              <Trans id='Uninstall app?' />
            </DialogTitle>
            <DialogDescription>
              <Trans id='Uninstall' /> <span className='mono'>{inst.metadata.name}</span>.
            </DialogDescription>
          </DialogHeader>
          <UninstallBody name={inst.metadata.name} onDone={() => setDeleteOpen(false)} del={del} />
        </DialogContent>
      </Dialog>
    </>
  );
}

function UninstallBody({
  name,
  onDone,
  del,
}: {
  name: string;
  onDone: () => void;
  del: ReturnType<typeof useDeleteAppInstance>;
}) {
  const toast = useToast();
  const [deleteData, setDeleteData] = useState(false);
  return (
    <>
      <div className='flex items-center gap-2 text-sm'>
        <Checkbox checked={deleteData} onCheckedChange={(v) => setDeleteData(!!v)} />
        <Trans id='Also delete persistent data' />
      </div>
      <DialogFooter>
        <Button variant='ghost' onClick={onDone}>
          <Trans id='Cancel' />
        </Button>
        <Button
          variant='danger'
          disabled={del.isPending}
          onClick={async () => {
            try {
              await del.mutateAsync({ name, deleteData });
              toast.success(i18n._('Uninstalled'), name);
              onDone();
            } catch (err) {
              toast.error(i18n._('Uninstall failed'), (err as Error)?.message);
            }
          }}
        >
          {del.isPending ? <Trans id='Uninstalling…' /> : <Trans id='Uninstall' />}
        </Button>
      </DialogFooter>
    </>
  );
}

// -----------------------------------------------------------------------------
interface ValueEntry {
  key: string;
  value: string;
}
interface StorageEntry {
  name: string;
  dataset: string;
  mountPath: string;
}

function InstallAppDialog({
  app,
  onOpenChange,
}: {
  app: App | null;
  onOpenChange: (v: boolean) => void;
}) {
  const create = useCreateAppInstance();
  const datasets = useDatasets();
  const toast = useToast();
  const [name, setName] = useState('');
  const [values, setValues] = useState<ValueEntry[]>([]);
  const [schemaValues, setSchemaValues] = useState<Record<string, unknown>>({});
  const [storages, setStorages] = useState<StorageEntry[]>([]);
  const [expose, setExpose] = useState<ExposureMode>('lan');

  // If the app declares a JSON-Schema, drive the Values tab from RJSF.
  const appSchema = (app?.spec as { schema?: unknown } | undefined)?.schema as
    | Record<string, unknown>
    | undefined;
  const hasSchema =
    !!appSchema && typeof appSchema === 'object' && Object.keys(appSchema).length > 0;

  // Seed schema defaults when the target app changes.
  useEffect(() => {
    if (!hasSchema) {
      setSchemaValues({});
      return;
    }
    const topDefault = (appSchema as { default?: unknown }).default;
    if (topDefault && typeof topDefault === 'object') {
      setSchemaValues({ ...(topDefault as Record<string, unknown>) });
    } else {
      setSchemaValues({});
    }
  }, [hasSchema, appSchema]);

  const reset = () => {
    setName('');
    setValues([]);
    setSchemaValues({});
    setStorages([]);
    setExpose('lan');
  };

  const submit = async () => {
    if (!app || !name) return;
    const body: AppInstanceCreateBody = {
      metadata: { name },
      spec: {
        app: app.metadata.name,
        version: app.spec.version,
        values: hasSchema
          ? schemaValues
          : Object.fromEntries(values.filter((v) => v.key).map((v) => [v.key, v.value])),
        storage: storages.length
          ? storages.map((s) => ({
              name: s.name,
              dataset: s.dataset || undefined,
              mountPath: s.mountPath || undefined,
            }))
          : undefined,
        network: {
          expose: [{ port: app.spec.requirements?.ports?.[0] ?? 80, advertise: expose }],
        },
      },
    };
    try {
      const resp = await create.mutateAsync(body);
      maybeTrackJobFromResponse(resp, `${i18n._('Install')} ${app.spec.displayName}`);
      toast.success(i18n._('App installed'), name);
      reset();
      onOpenChange(false);
    } catch (err) {
      toast.error(i18n._('Install failed'), (err as Error)?.message);
    }
  };

  return (
    <Dialog open={!!app} onOpenChange={onOpenChange}>
      <DialogContent className='max-w-lg'>
        <DialogHeader>
          <DialogTitle>
            <Trans id='Install' /> {app?.spec.displayName}
          </DialogTitle>
          <DialogDescription>
            v{app?.spec.version}
            {app?.spec.description ? ` — ${app.spec.description}` : ''}
          </DialogDescription>
        </DialogHeader>

        <Tabs defaultValue='basic'>
          <TabsList>
            <TabsTrigger value='basic'>
              <Trans id='Basic' />
            </TabsTrigger>
            <TabsTrigger value='values'>
              <Trans id='Values' />
            </TabsTrigger>
            <TabsTrigger value='storage'>
              <Trans id='Storage' />
            </TabsTrigger>
            <TabsTrigger value='network'>
              <Trans id='Network' />
            </TabsTrigger>
          </TabsList>

          <TabsContent value='basic' className='pt-3'>
            <FormField label={i18n._('Instance name')} required>
              <Input
                value={name}
                onChange={(e) => setName(e.target.value)}
                placeholder='my-instance'
              />
            </FormField>
          </TabsContent>

          <TabsContent value='values' className='pt-3'>
            {hasSchema ? (
              <>
                <div className='text-xs text-foreground-subtle mb-2'>
                  <Trans id='Configuration schema provided by the app.' />
                </div>
                <SchemaForm schema={appSchema} formData={schemaValues} onChange={setSchemaValues} />
              </>
            ) : (
              <>
                <div className='text-xs text-foreground-subtle mb-2'>
                  <Trans id='Free-form key/value overrides (Helm values). For structured forms, see docs.' />
                </div>
                <ul className='flex flex-col gap-1 mb-2'>
                  {values.map((v, i) => (
                    <li key={i} className='flex gap-2'>
                      <Input
                        placeholder={i18n._('key')}
                        value={v.key}
                        onChange={(e) => {
                          const n = [...values];
                          n[i] = { ...v, key: e.target.value };
                          setValues(n);
                        }}
                      />
                      <Input
                        placeholder={i18n._('value')}
                        value={v.value}
                        onChange={(e) => {
                          const n = [...values];
                          n[i] = { ...v, value: e.target.value };
                          setValues(n);
                        }}
                      />
                      <Button
                        size='sm'
                        variant='ghost'
                        onClick={() => setValues(values.filter((_, j) => j !== i))}
                      >
                        <X size={11} />
                      </Button>
                    </li>
                  ))}
                </ul>
                <Button
                  variant='ghost'
                  size='sm'
                  onClick={() => setValues([...values, { key: '', value: '' }])}
                >
                  <Trans id='Add value' />
                </Button>
              </>
            )}
          </TabsContent>

          <TabsContent value='storage' className='pt-3'>
            <ul className='flex flex-col gap-2 mb-2'>
              {storages.map((s, i) => (
                <li key={i} className='grid grid-cols-3 gap-2 items-end'>
                  <FormField label={i18n._('Name')}>
                    <Input
                      value={s.name}
                      onChange={(e) => {
                        const n = [...storages];
                        n[i] = { ...s, name: e.target.value };
                        setStorages(n);
                      }}
                    />
                  </FormField>
                  <FormField label={i18n._('Dataset')}>
                    <Select
                      value={s.dataset}
                      onValueChange={(v) => {
                        const n = [...storages];
                        n[i] = { ...s, dataset: v };
                        setStorages(n);
                      }}
                    >
                      <SelectTrigger>
                        <SelectValue placeholder={i18n._('select')} />
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
                  <div className='flex gap-2'>
                    <Input
                      placeholder='/data'
                      value={s.mountPath}
                      onChange={(e) => {
                        const n = [...storages];
                        n[i] = { ...s, mountPath: e.target.value };
                        setStorages(n);
                      }}
                    />
                    <Button
                      size='sm'
                      variant='ghost'
                      onClick={() => setStorages(storages.filter((_, j) => j !== i))}
                    >
                      <X size={11} />
                    </Button>
                  </div>
                </li>
              ))}
            </ul>
            <Button
              variant='ghost'
              size='sm'
              onClick={() =>
                setStorages([
                  ...storages,
                  { name: `data-${storages.length + 1}`, dataset: '', mountPath: '' },
                ])
              }
            >
              <Trans id='Add storage' />
            </Button>
          </TabsContent>

          <TabsContent value='network' className='pt-3'>
            <FormField label={i18n._('Exposure')}>
              <Select value={expose} onValueChange={(v) => setExpose(v as ExposureMode)}>
                <SelectTrigger>
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectItem value='mdns'>mdns (local)</SelectItem>
                  <SelectItem value='lan'>lan</SelectItem>
                  <SelectItem value='reverseProxy'>reverse proxy</SelectItem>
                  <SelectItem value='internet'>internet</SelectItem>
                </SelectContent>
              </Select>
            </FormField>
          </TabsContent>
        </Tabs>

        <DialogFooter>
          <Button variant='ghost' onClick={() => onOpenChange(false)}>
            <Trans id='Cancel' />
          </Button>
          <Button variant='primary' disabled={!name || create.isPending} onClick={submit}>
            {create.isPending ? <Trans id='Installing…' /> : <Trans id='Install' />}
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
