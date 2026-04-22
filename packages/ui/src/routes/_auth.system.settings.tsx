import { useSystemSettings, useUpdateSystemSettings } from '@/api/system-settings';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
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
import { Skeleton } from '@/components/ui/skeleton';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { ApiError, api } from '@/lib/api';
import { i18n } from '@/lib/i18n';
import { maybeTrackJobFromResponse } from '@/stores/jobs';
import { Trans } from '@lingui/react';
import { createFileRoute } from '@tanstack/react-router';
import { AlertTriangle, Settings2 } from 'lucide-react';
import { useEffect, useState } from 'react';

export const Route = createFileRoute('/_auth/system/settings')({
  component: SettingsPage,
});

function SettingsPage() {
  const { canMutate } = useAuth();
  const q = useSystemSettings();
  const save = useUpdateSystemSettings();
  const toast = useToast();
  const mayMutate = canMutate();

  const [hostname, setHostname] = useState('');
  const [timezone, setTimezone] = useState('');
  const [locale, setLocale] = useState('');
  const [ntpServers, setNtpServers] = useState('');
  const [smtpHost, setSmtpHost] = useState('');
  const [smtpPort, setSmtpPort] = useState('587');
  const [smtpFrom, setSmtpFrom] = useState('');
  const [smtpEncryption, setSmtpEncryption] = useState<'none' | 'starttls' | 'tls'>('starttls');

  useEffect(() => {
    if (!q.data) return;
    const s = q.data.spec;
    setHostname(s.hostname ?? '');
    setTimezone(s.timezone ?? '');
    setLocale(s.locale ?? '');
    setNtpServers((s.ntp?.servers ?? []).join(', '));
    setSmtpHost(s.smtp?.host ?? '');
    setSmtpPort(String(s.smtp?.port ?? 587));
    setSmtpFrom(s.smtp?.from ?? '');
    setSmtpEncryption(s.smtp?.encryption ?? 'starttls');
  }, [q.data]);

  const submit = async () => {
    try {
      await save.mutateAsync({
        hostname: hostname || undefined,
        timezone: timezone || undefined,
        locale: locale || undefined,
        ntp: ntpServers
          ? {
              enabled: true,
              servers: ntpServers
                .split(',')
                .map((s) => s.trim())
                .filter(Boolean),
            }
          : undefined,
        smtp:
          smtpHost && smtpFrom
            ? {
                host: smtpHost,
                port: Number.parseInt(smtpPort, 10) || 587,
                encryption: smtpEncryption,
                from: smtpFrom,
              }
            : undefined,
      });
      toast.success(i18n._('Settings saved'));
    } catch (e) {
      toast.error(i18n._('Save failed'), (e as Error).message);
    }
  };

  return (
    <>
      <PageHeader
        title={i18n._('Settings')}
        subtitle={i18n._('System-wide configuration.')}
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={submit} disabled={save.isPending}>
              {save.isPending ? <Trans id='Saving…' /> : <Trans id='Save' />}
            </Button>
          ) : null
        }
      />
      {q.isLoading ? (
        <Skeleton className='h-48' />
      ) : q.isError ? (
        <EmptyState
          icon={<Settings2 size={28} />}
          title={i18n._('Unable to load settings')}
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>{i18n._('Retry')}</Button>}
        />
      ) : (
        <div className='flex flex-col gap-5'>
          <section className='flex flex-col gap-3'>
            <h2 className='text-md font-semibold'>
              <Trans id='General' />
            </h2>
            <div className='grid grid-cols-2 gap-3'>
              <FormField label={i18n._('Hostname')}>
                <Input
                  value={hostname}
                  onChange={(e) => setHostname(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label={i18n._('Timezone')}>
                <Input
                  value={timezone}
                  onChange={(e) => setTimezone(e.target.value)}
                  placeholder='Europe/Brussels'
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label={i18n._('Locale')}>
                <Input
                  value={locale}
                  onChange={(e) => setLocale(e.target.value)}
                  placeholder='en_US.UTF-8'
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label={i18n._('NTP servers')} hint={i18n._('Comma-separated')}>
                <Input
                  value={ntpServers}
                  onChange={(e) => setNtpServers(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
            </div>
          </section>
          <section className='flex flex-col gap-3'>
            <h2 className='text-md font-semibold'>SMTP</h2>
            <div className='grid grid-cols-2 gap-3'>
              <FormField label={i18n._('Host')}>
                <Input
                  value={smtpHost}
                  onChange={(e) => setSmtpHost(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label={i18n._('Port')}>
                <Input
                  type='number'
                  value={smtpPort}
                  onChange={(e) => setSmtpPort(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label={i18n._('From')}>
                <Input
                  type='email'
                  value={smtpFrom}
                  onChange={(e) => setSmtpFrom(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label={i18n._('Encryption')}>
                <select
                  value={smtpEncryption}
                  onChange={(e) => setSmtpEncryption(e.target.value as typeof smtpEncryption)}
                  disabled={!mayMutate}
                  className='rounded-sm border border-border bg-surface px-2 py-1.5 text-sm'
                >
                  <option value='none'>none</option>
                  <option value='starttls'>starttls</option>
                  <option value='tls'>tls</option>
                </select>
              </FormField>
            </div>
          </section>
          {q.data?.status?.appliedAt && (
            <div className='text-xs text-foreground-subtle mono'>
              <Trans id='Last applied' />: {q.data.status.appliedAt}
            </div>
          )}

          {mayMutate && (
            <FactoryResetSection hostname={hostname || q.data?.spec.hostname || 'novanas'} />
          )}
        </div>
      )}
    </>
  );
}

// -----------------------------------------------------------------------------
// Factory reset — issue #33
// -----------------------------------------------------------------------------
type ResetTier = 'soft' | 'config' | 'full';

function getTierCopy(tier: ResetTier): {
  title: string;
  summary: string;
  cta: string;
  danger: boolean;
  typeConfirm: boolean;
} {
  switch (tier) {
    case 'soft':
      return {
        title: i18n._('Soft reset'),
        summary: i18n._(
          'Restart system services and clear transient caches. User data, datasets and pools are preserved.'
        ),
        cta: i18n._('Soft reset'),
        danger: false,
        typeConfirm: false,
      };
    case 'config':
      return {
        title: i18n._('Reset configuration'),
        summary: i18n._(
          'Reverts all settings (network, identity, SMTP, alerts…) to defaults. Datasets and pools are preserved, but apps/VMs may become unreachable until reconfigured.'
        ),
        cta: i18n._('Reset configuration'),
        danger: true,
        typeConfirm: false,
      };
    case 'full':
      return {
        title: i18n._('Factory reset — erase everything'),
        summary: i18n._(
          'Destroys all pools, datasets, VMs, apps and configuration. The appliance will reboot into the first-run wizard.'
        ),
        cta: i18n._('Factory reset'),
        danger: true,
        typeConfirm: true,
      };
  }
}

function FactoryResetSection({ hostname }: { hostname: string }) {
  const [pending, setPending] = useState<ResetTier | null>(null);
  return (
    <section className='flex flex-col gap-3 mt-6 border border-danger/40 rounded-md p-4 bg-danger/5'>
      <div className='flex items-center gap-2'>
        <AlertTriangle size={16} className='text-danger' />
        <h2 className='text-md font-semibold text-danger'>
          <Trans id='Danger zone' />
        </h2>
      </div>
      <p className='text-xs text-foreground-muted max-w-prose'>
        <Trans id='These actions are destructive. Each tier has its own confirmation flow.' />
      </p>
      <div className='grid grid-cols-1 md:grid-cols-3 gap-3'>
        {(['soft', 'config', 'full'] as ResetTier[]).map((tier) => {
          const copy = getTierCopy(tier);
          return (
            <div key={tier} className='border border-border rounded-sm p-3 flex flex-col gap-2'>
              <div className='font-medium text-sm'>{copy.title}</div>
              <div className='text-xs text-foreground-subtle flex-1'>{copy.summary}</div>
              <Button
                variant={copy.danger ? 'danger' : 'default'}
                size='sm'
                onClick={() => setPending(tier)}
              >
                {copy.cta}
              </Button>
            </div>
          );
        })}
      </div>
      <FactoryResetDialog tier={pending} hostname={hostname} onClose={() => setPending(null)} />
    </section>
  );
}

function FactoryResetDialog({
  tier,
  hostname,
  onClose,
}: {
  tier: ResetTier | null;
  hostname: string;
  onClose: () => void;
}) {
  const [typed, setTyped] = useState('');
  const [submitting, setSubmitting] = useState(false);
  const [notImplemented, setNotImplemented] = useState(false);
  const toast = useToast();

  useEffect(() => {
    // Reset confirmation state whenever we switch between tiers or re-open.
    void tier;
    setTyped('');
    setNotImplemented(false);
  }, [tier]);

  if (!tier) return null;
  const copy = getTierCopy(tier);
  const requiresType = copy.typeConfirm;
  const canSubmit = !submitting && (!requiresType || typed === hostname);

  const submit = async () => {
    setSubmitting(true);
    setNotImplemented(false);
    try {
      const resp = await api.post('/system/reset', undefined, { searchParams: { tier } });
      maybeTrackJobFromResponse(resp, copy.title);
      toast.success(i18n._('Reset requested'), copy.title);
      onClose();
    } catch (err) {
      if (err instanceof ApiError && err.status === 501) {
        setNotImplemented(true);
      } else {
        toast.error(i18n._('Reset failed'), (err as Error)?.message);
      }
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <Dialog open={!!tier} onOpenChange={(v) => !v && onClose()}>
      <DialogContent>
        <DialogHeader>
          <DialogTitle className='flex items-center gap-2 text-danger'>
            <AlertTriangle size={16} /> {copy.title}
          </DialogTitle>
          <DialogDescription>{copy.summary}</DialogDescription>
        </DialogHeader>
        {notImplemented ? (
          <div className='text-sm border border-warning/40 bg-warning/10 rounded-sm p-3'>
            <Trans id='This endpoint is not yet implemented on the appliance.' />{' '}
            <a
              className='underline text-foreground'
              href='https://github.com/novanas/novanas/issues/33'
              target='_blank'
              rel='noreferrer'
            >
              <Trans id='Track issue #33' />
            </a>{' '}
            <Trans id='for progress.' />
          </div>
        ) : (
          requiresType && (
            <div className='flex flex-col gap-2 text-sm'>
              <div>
                <Trans id='Type the hostname' />{' '}
                <span className='mono text-foreground'>{hostname}</span> <Trans id='to confirm.' />
              </div>
              <Input
                value={typed}
                onChange={(e) => setTyped(e.target.value)}
                placeholder={hostname}
                autoFocus
              />
            </div>
          )
        )}
        <DialogFooter>
          <Button variant='ghost' onClick={onClose}>
            <Trans id='Cancel' />
          </Button>
          {!notImplemented && (
            <Button variant='danger' disabled={!canSubmit} onClick={submit}>
              {submitting ? <Trans id='Submitting…' /> : copy.cta}
            </Button>
          )}
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
