import {
  useCheckForUpdates,
  useSaveUpdatePolicy,
  useUpdateHistory,
  useUpdatePolicy,
} from '@/api/update-policy';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { StatusDot } from '@/components/common/status-dot';
import { Badge } from '@/components/ui/badge';
import { Button } from '@/components/ui/button';
import { Checkbox } from '@/components/ui/checkbox';
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
import { createFileRoute } from '@tanstack/react-router';
import { Download, RefreshCw } from 'lucide-react';
import { useEffect, useState } from 'react';

export const Route = createFileRoute('/_auth/system/updates')({
  component: UpdatesPage,
});

function UpdatesPage() {
  const { canMutate } = useAuth();
  const q = useUpdatePolicy();
  const save = useSaveUpdatePolicy();
  const check = useCheckForUpdates();
  const history = useUpdateHistory();
  const toast = useToast();
  const mayMutate = canMutate();

  const [channel, setChannel] = useState<'stable' | 'beta' | 'edge' | 'manual'>('stable');
  const [autoUpdate, setAutoUpdate] = useState(false);
  const [autoReboot, setAutoReboot] = useState(false);
  const [cron, setCron] = useState('');
  const [durationMinutes, setDurationMinutes] = useState('60');

  useEffect(() => {
    if (!q.data) return;
    const s = q.data.spec;
    setChannel(s.channel);
    setAutoUpdate(!!s.autoUpdate);
    setAutoReboot(!!s.autoReboot);
    setCron(s.maintenanceWindow?.cron ?? '');
    setDurationMinutes(String(s.maintenanceWindow?.durationMinutes ?? 60));
  }, [q.data]);

  const submit = async () => {
    try {
      await save.mutateAsync({
        channel,
        autoUpdate,
        autoReboot,
        maintenanceWindow: cron
          ? { cron, durationMinutes: Number.parseInt(durationMinutes, 10) || 60 }
          : undefined,
      });
      toast.success(i18n._('Update policy saved'));
    } catch (e) {
      toast.error(i18n._('Save failed'), (e as Error).message);
    }
  };

  const runCheck = async () => {
    try {
      await check.mutateAsync();
      toast.success(i18n._('Checking for updates…'));
    } catch (e) {
      toast.error(i18n._('Check failed'), (e as Error).message);
    }
  };

  return (
    <>
      <PageHeader
        title={i18n._('Updates')}
        subtitle={i18n._('Channel, maintenance window, and version history.')}
        actions={
          mayMutate ? (
            <div className='flex gap-2'>
              <Button variant='ghost' onClick={runCheck} disabled={check.isPending}>
                <RefreshCw size={13} /> <Trans id='Check for updates' />
              </Button>
              <Button variant='primary' onClick={submit} disabled={save.isPending}>
                {save.isPending ? <Trans id='Saving…' /> : <Trans id='Save' />}
              </Button>
            </div>
          ) : null
        }
      />
      {q.isLoading ? (
        <Skeleton className='h-48' />
      ) : q.isError ? (
        <EmptyState
          icon={<Download size={28} />}
          title={i18n._('Unable to load update policy')}
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>{i18n._('Retry')}</Button>}
        />
      ) : (
        <div className='flex flex-col gap-5'>
          <section className='grid grid-cols-3 gap-3 text-xs'>
            <div className='border border-border rounded-md p-3'>
              <div className='text-foreground-subtle uppercase tracking-wider mb-1'>
                <Trans id='Phase' />
              </div>
              <div className='flex items-center gap-2'>
                <StatusDot
                  tone={
                    q.data?.status?.phase === 'Idle'
                      ? 'ok'
                      : q.data?.status?.phase === 'Failed'
                        ? 'err'
                        : 'warn'
                  }
                />
                <span className='mono'>{q.data?.status?.phase ?? 'Idle'}</span>
              </div>
            </div>
            <div className='border border-border rounded-md p-3'>
              <div className='text-foreground-subtle uppercase tracking-wider mb-1'>
                <Trans id='Current' />
              </div>
              <div className='mono'>{q.data?.status?.currentVersion ?? '—'}</div>
            </div>
            <div className='border border-border rounded-md p-3'>
              <div className='text-foreground-subtle uppercase tracking-wider mb-1'>
                <Trans id='Available' />
              </div>
              <div className='mono'>
                {q.data?.status?.availableVersion ? (
                  <Badge>{q.data.status.availableVersion}</Badge>
                ) : (
                  '—'
                )}
              </div>
            </div>
          </section>
          <section className='flex flex-col gap-3'>
            <h2 className='text-md font-semibold'>
              <Trans id='Policy' />
            </h2>
            <div className='grid grid-cols-2 gap-3'>
              <FormField label={i18n._('Channel')}>
                <Select
                  value={channel}
                  onValueChange={(v) => setChannel(v as typeof channel)}
                  disabled={!mayMutate}
                >
                  <SelectTrigger>
                    <SelectValue />
                  </SelectTrigger>
                  <SelectContent>
                    <SelectItem value='stable'>stable</SelectItem>
                    <SelectItem value='beta'>beta</SelectItem>
                    <SelectItem value='edge'>edge</SelectItem>
                    <SelectItem value='manual'>manual</SelectItem>
                  </SelectContent>
                </Select>
              </FormField>
              <div className='flex items-end gap-4'>
                <div className='flex items-center gap-2 text-sm'>
                  <Checkbox
                    checked={autoUpdate}
                    onCheckedChange={(v) => setAutoUpdate(!!v)}
                    disabled={!mayMutate}
                  />
                  <span>
                    <Trans id='Auto-update' />
                  </span>
                </div>
                <div className='flex items-center gap-2 text-sm'>
                  <Checkbox
                    checked={autoReboot}
                    onCheckedChange={(v) => setAutoReboot(!!v)}
                    disabled={!mayMutate}
                  />
                  <span>
                    <Trans id='Auto-reboot' />
                  </span>
                </div>
              </div>
              <FormField label={i18n._('Maintenance cron')} hint={i18n._('e.g. 0 3 * * SUN')}>
                <Input
                  value={cron}
                  onChange={(e) => setCron(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label={i18n._('Window duration (minutes)')}>
                <Input
                  type='number'
                  value={durationMinutes}
                  onChange={(e) => setDurationMinutes(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
            </div>
          </section>
          <section className='flex flex-col gap-2'>
            <h2 className='text-md font-semibold'>
              <Trans id='History' />
            </h2>
            {history.isLoading ? (
              <Skeleton className='h-24' />
            ) : history.isError ? (
              <EmptyState title={i18n._('Unable to load history')} />
            ) : (history.data?.length ?? 0) === 0 ? (
              <EmptyState title={i18n._('No update history')} />
            ) : (
              <div className='border border-border rounded-md overflow-hidden'>
                <Table>
                  <TableHead>
                    <tr>
                      <TableHeaderCell>
                        <Trans id='Version' />
                      </TableHeaderCell>
                      <TableHeaderCell>
                        <Trans id='Applied at' />
                      </TableHeaderCell>
                      <TableHeaderCell>
                        <Trans id='Status' />
                      </TableHeaderCell>
                      <TableHeaderCell>
                        <Trans id='Notes' />
                      </TableHeaderCell>
                    </tr>
                  </TableHead>
                  <TableBody>
                    {history.data!.map((h) => (
                      <TableRow key={h.id}>
                        <TableCell className='mono text-xs'>{h.version}</TableCell>
                        <TableCell className='mono text-xs'>{h.appliedAt}</TableCell>
                        <TableCell>
                          <Badge>{h.status}</Badge>
                        </TableCell>
                        <TableCell className='text-xs'>{h.notes ?? '—'}</TableCell>
                      </TableRow>
                    ))}
                  </TableBody>
                </Table>
              </div>
            )}
          </section>
        </div>
      )}
    </>
  );
}
