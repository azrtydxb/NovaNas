import { useSystemSettings, useUpdateSystemSettings } from '@/api/system-settings';
import { EmptyState } from '@/components/common/empty-state';
import { FormField } from '@/components/common/form-field';
import { PageHeader } from '@/components/common/page-header';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Skeleton } from '@/components/ui/skeleton';
import { useAuth } from '@/hooks/use-auth';
import { useToast } from '@/hooks/use-toast';
import { createFileRoute } from '@tanstack/react-router';
import { Settings2 } from 'lucide-react';
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
      toast.success('Settings saved');
    } catch (e) {
      toast.error('Save failed', (e as Error).message);
    }
  };

  return (
    <>
      <PageHeader
        title='Settings'
        subtitle='System-wide configuration.'
        actions={
          mayMutate ? (
            <Button variant='primary' onClick={submit} disabled={save.isPending}>
              {save.isPending ? 'Saving…' : 'Save'}
            </Button>
          ) : null
        }
      />
      {q.isLoading ? (
        <Skeleton className='h-48' />
      ) : q.isError ? (
        <EmptyState
          icon={<Settings2 size={28} />}
          title='Unable to load settings'
          description={(q.error as Error)?.message}
          action={<Button onClick={() => q.refetch()}>Retry</Button>}
        />
      ) : (
        <div className='flex flex-col gap-5'>
          <section className='flex flex-col gap-3'>
            <h2 className='text-md font-semibold'>General</h2>
            <div className='grid grid-cols-2 gap-3'>
              <FormField label='Hostname'>
                <Input
                  value={hostname}
                  onChange={(e) => setHostname(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label='Timezone'>
                <Input
                  value={timezone}
                  onChange={(e) => setTimezone(e.target.value)}
                  placeholder='Europe/Brussels'
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label='Locale'>
                <Input
                  value={locale}
                  onChange={(e) => setLocale(e.target.value)}
                  placeholder='en_US.UTF-8'
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label='NTP servers' hint='Comma-separated'>
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
              <FormField label='Host'>
                <Input
                  value={smtpHost}
                  onChange={(e) => setSmtpHost(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label='Port'>
                <Input
                  type='number'
                  value={smtpPort}
                  onChange={(e) => setSmtpPort(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label='From'>
                <Input
                  type='email'
                  value={smtpFrom}
                  onChange={(e) => setSmtpFrom(e.target.value)}
                  disabled={!mayMutate}
                />
              </FormField>
              <FormField label='Encryption'>
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
              Last applied: {q.data.status.appliedAt}
            </div>
          )}
        </div>
      )}
    </>
  );
}
